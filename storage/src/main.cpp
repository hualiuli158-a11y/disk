#include "object_storage_service.h"

#include <grpcpp/grpcpp.h>
#include <grpcpp/health_check_service_interface.h>
#include <grpc/grpc_security_constants.h>

#include <atomic>
#include <chrono>
#include <csignal>
#include <exception>
#include <fstream>
#include <iostream>
#include <iterator>
#include <memory>
#include <stdexcept>
#include <string>
#include <thread>

volatile std::sig_atomic_t stop_requested = 0;
void RequestStop(int) { stop_requested = 1; }

std::string ReadFile(const char* path)
{
    std::ifstream input(path, std::ios::binary);
    if (!input) throw std::runtime_error(std::string("cannot read TLS file: ") + path);
    return {std::istreambuf_iterator<char>(input), std::istreambuf_iterator<char>()};
}


//  ./grpc_storage_node IP:Port 路径
int main(int argc, char* argv[])
{
    if (argc != 3 && argc != 5 && argc != 6)
    {
        std::cerr << "usage: grpc_storage_node <listen-address> <storage-root> "
        "[server-cert.pem server-key.pem [client-ca.pem]]\n";
        return 2;
    }
    try
    {
        if (std::signal(SIGINT, RequestStop) == SIG_ERR || std::signal(SIGTERM, RequestStop) == SIG_ERR)
            throw std::runtime_error("could not install signal handler");

        ObjectStorageService service(argv[2]);
        std::shared_ptr<grpc::ServerCredentials> credentials;
        if (argc == 3)
        {
            credentials = grpc::InsecureServerCredentials();
        }
        else
        {
            grpc::SslServerCredentialsOptions options;
            options.pem_key_cert_pairs.push_back({ReadFile(argv[4]), ReadFile(argv[3])});
            if (argc == 6)
            {
                options.pem_root_certs = ReadFile(argv[5]);
                options.client_certificate_request =
                    GRPC_SSL_REQUEST_AND_REQUIRE_CLIENT_CERTIFICATE_AND_VERIFY;
            }
            credentials = grpc::SslServerCredentials(options);
        }
        grpc::EnableDefaultHealthCheckService(true);

        grpc::ServerBuilder builder;// 1. 创建建造者对象
        builder.AddListeningPort(argv[1], credentials);// 2. 绑定监听地址与安全凭证（如不加密使用 InsecureServerCredentials）
        builder.RegisterService(&service); // 3. 注册你的 RPC 服务
        auto server = builder.BuildAndStart();// 4. 构建并启动真正的 Server 网络实例化对象
        if (!server) throw std::runtime_error("could not start gRPC server");
        auto* health = server->GetHealthCheckService();
        if (!health) throw std::runtime_error("gRPC health service is unavailable");
        health->SetServingStatus("", true);

        std::cout << "gRPC storage listening on " << argv[1]
                  << ", root=" << argv[2] << std::endl;
        std::thread stop_watcher([&] {
            while (!stop_requested)
                std::this_thread::sleep_for(std::chrono::milliseconds(50));
            health->Shutdown();
            server->Shutdown();
        });
        server->Wait();
        stop_requested = 1;
        stop_watcher.join();
        return 0;
    }
    catch (const std::exception& error)
    {
        std::cerr << "gRPC storage failed: " << error.what() << '\n';
        return 1;
    }
}
