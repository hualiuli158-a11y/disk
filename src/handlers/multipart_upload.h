#pragma once

#include "common.h"
#include "http_server.h"
#include "multipart_parser.h"

#include <cstddef>
#include <deque>
#include <memory>
#include <string>

namespace disk::handler {

inline std::string Parameter(const std::string &header, const std::string &key)
{
    std::size_t position = header.find(';');
    while (position != std::string::npos && ++position < header.size())
    {
        while (position < header.size() && header[position] == ' ')
            ++position;
        const auto equal = header.find('=', position);
        if (equal == std::string::npos)
            return {};
        auto name = header.substr(position, equal - position);
        while (!name.empty() && name.back() == ' ')
            name.pop_back();
        position = equal + 1;
        std::string value;
        if (position < header.size() && header[position] == '"')
        {
            ++position;
            bool closed = false;
            while (position < header.size())
            {
                char c = header[position++];
                if (c == '"')
                {
                    closed = true;
                    break;
                }
                if (c == '\\' && position < header.size())
                    c = header[position++];
                value += c;
            }
            if (!closed)
                throw Error(400, "Malformed quoted parameter");
        }
        else
        {
            const auto end = header.find(';', position);
            value = header.substr(position, end - position);
            position = end;
        }
        if (name == key)
            return value;
        position = header.find(';', position);
    }
    return {};
}

class MultipartUpload
{
public:
    MultipartUpload(std::shared_ptr<BodyPipe> pipe, const std::string &boundary) : pipe_(std::move(pipe))
    {
        if (boundary.empty() || boundary.size() > 200 || boundary.find_first_of("\r\n") != std::string::npos)
            throw Error(400, "Invalid multipart boundary");
        callbacks_.on_part_data_begin = Begin;
        callbacks_.on_header_field = HeaderField;
        callbacks_.on_header_value = HeaderValue;
        callbacks_.on_headers_complete = HeadersComplete;
        callbacks_.on_part_data = Data;
        callbacks_.on_part_data_end = End;
        callbacks_.on_body_end = Done;
        const auto delimiter = "--" + boundary;
        parser_.reset(multipart_parser_init(delimiter.c_str(), &callbacks_));
        if (!parser_)
            throw std::bad_alloc();
        multipart_parser_set_data(parser_.get(), this);
        while (!fileStarted_)
        {
            if (!ParseNext())
                throw Error(400, "Missing upload file");
        }
    }
    std::uint64_t folder = 0;
    std::string name;
    bool Read(std::string &block)
    {
        while (pending_.empty())
        {
            if (!ParseNext())
            {
                if (!done_ || !fileDone_)
                    throw Error(400, "Incomplete multipart upload");
                return false;
            }
        }
        block = std::move(pending_.front());
        pending_.pop_front();
        return true;
    }

private:
    static MultipartUpload &Self(multipart_parser *parser)
    {
        return *static_cast<MultipartUpload *>(multipart_parser_get_data(parser));
    }
    template <class Operation> static int Invoke(multipart_parser *parser, Operation operation) noexcept
    {
        auto &self = Self(parser);
        try
        {
            operation(self);
            return 0;
        }
        catch (...)
        {
            self.error_ = std::current_exception();
            return 1;
        }
    }
    static int Begin(multipart_parser *p)
    {
        return Invoke(p,
                      [](auto &s)
                      {
                          if (++s.parts_ > 2)
                              throw Error(400, "Unexpected multipart part");
                          s.field_.clear();
                          s.value_.clear();
                          s.disposition_.clear();
                          s.fieldData_.clear();
                          s.inValue_ = false;
                      });
    }
    void SaveHeader()
    {
        if (field_ == "Content-Disposition" || field_ == "content-disposition")
            disposition_ = value_;
        field_.clear();
        value_.clear();
    }
    static int HeaderField(multipart_parser *p, const char *at, size_t length)
    {
        return Invoke(p,
                      [&](auto &s)
                      {
                          if (s.inValue_)
                          {
                              s.SaveHeader();
                              s.inValue_ = false;
                          }
                          s.field_.append(at, length);
                          if ((s.headerBytes_ += length) > 8192)
                              throw Error(400, "Multipart headers too large");
                      });
    }
    static int HeaderValue(multipart_parser *p, const char *at, size_t length)
    {
        return Invoke(p,
                      [&](auto &s)
                      {
                          s.inValue_ = true;
                          s.value_.append(at, length);
                          if ((s.headerBytes_ += length) > 8192)
                              throw Error(400, "Multipart headers too large");
                      });
    }
    static int HeadersComplete(multipart_parser *p)
    {
        return Invoke(p,
                      [](auto &s)
                      {
                          s.SaveHeader();
                          s.partName_ = Parameter(s.disposition_, "name");
                          if (s.partName_ == "folder_id" && s.parts_ == 1)
                              return;
                          if (s.partName_ != "file" || s.parts_ != 2 || !s.folder)
                              throw Error(400, "Expected folder_id followed by file");
                          s.name = Parameter(s.disposition_, "filename");
                          s.fileStarted_ = true;
                      });
    }
    static int Data(multipart_parser *p, const char *at, size_t length)
    {
        return Invoke(p,
                      [&](auto &s)
                      {
                          if (s.partName_ == "folder_id")
                          {
                              if (s.fieldData_.size() + length > 20)
                                  throw Error(400, "Invalid folder identifier");
                              s.fieldData_.append(at, length);
                          }
                          else
                          {
                              while (length)
                              {
                                  if (s.pending_.empty() || s.pending_.back().size() == 65536)
                                      s.pending_.emplace_back();
                                  const auto take = std::min(length, 65536 - s.pending_.back().size());
                                  s.pending_.back().append(at, take);
                                  at += take;
                                  length -= take;
                              }
                          }
                      });
    }
    static int End(multipart_parser *p)
    {
        return Invoke(p,
                      [](auto &s)
                      {
                          if (s.partName_ == "folder_id")
                              s.folder = Id(s.fieldData_);
                          else
                              s.fileDone_ = true;
                      });
    }
    static int Done(multipart_parser *p)
    {
        return Invoke(p,
                      [](auto &s)
                      {
                          s.done_ = true;
                      });
    }
    bool ParseNext()
    {
        std::string input;
        if (!pipe_->Pop(input))
            return false;
        const auto parsed = multipart_parser_execute(parser_.get(), input.data(), input.size());
        if (error_)
            std::rethrow_exception(error_);
        if (parsed != input.size())
            throw Error(400, "Malformed multipart upload");
        return true;
    }
    std::shared_ptr<BodyPipe> pipe_;
    multipart_parser_settings callbacks_{};
    std::unique_ptr<multipart_parser, decltype(&multipart_parser_free)> parser_{nullptr,
                                                                                multipart_parser_free};
    std::deque<std::string> pending_;
    std::exception_ptr error_;
    std::string field_, value_, disposition_, fieldData_, partName_;
    std::size_t headerBytes_ = 0;
    int parts_ = 0;
    bool inValue_ = false, fileStarted_ = false, fileDone_ = false, done_ = false;
};

} // namespace disk::handler
