#pragma once

#include "gin/gin.h"
#include "common.h"
#include <map>
#include <mutex>

namespace disk::middleware {

inline gin::HandlerFunc UploadLimits(std::uint64_t max_bytes)
{
    struct State
    {
        std::mutex mutex;
        std::map<std::uint64_t, unsigned> active;
    };
    auto state = std::make_shared<State>();
    return [state, max_bytes](gin::Context& context) {
        const auto declared = context.GetHeader("x-upload-size");
        if (!declared.empty() && ParseUploadSize(declared) > max_bytes)
            throw Error(413, "File exceeds upload limit");
        const auto user = std::any_cast<std::uint64_t>(context.MustGet("user_id"));
        struct Reservation
        {
            std::shared_ptr<State> state;
            std::uint64_t user;
            bool acquired = false;
            ~Reservation()
            {
                if (!acquired) return;
                const std::lock_guard<std::mutex> lock(state->mutex);
                auto found = state->active.find(user);
                if (--found->second == 0) state->active.erase(found);
            }
        };
        auto reservation = std::make_shared<Reservation>(Reservation{state, user});
        {
            const std::lock_guard<std::mutex> lock(state->mutex);
            auto& count = state->active[user];
            if (count >= 2) throw Error(429, "Concurrent upload limit reached");
            ++count;
            reservation->acquired = true;
        }
        context.OnFinish([reservation] {});
        context.Next();
    };
}

} // namespace disk::middleware
