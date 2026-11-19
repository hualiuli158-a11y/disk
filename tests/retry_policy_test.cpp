#include "repositories/repository_support.h"
#include <stdexcept>
#include <iostream>
void Check(bool ok) { if (!ok) throw std::runtime_error("retry policy regression"); }
int main() {
    unsigned calls = 0, active = 0;
    auto operation = [&]() {
        struct Scope { unsigned& active; ~Scope() { --active; } } scope{active};
        Check(active++ == 0); // The failed attempt must unwind before retry.
        if (++calls < 3) throw boost::system::system_error(boost::mysql::common_server_errc::er_lock_deadlock);
        return 42;
    };
    Check(disk::RetryDeadlockedTransaction(operation) == 42 && calls == 3 && active == 0);
    calls = 0;
    try {
        disk::RetryDeadlockedTransaction([&]() -> int { ++calls; throw boost::system::system_error(boost::mysql::common_server_errc::er_lock_deadlock); });
        Check(false);
    } catch (const boost::system::system_error&) { Check(calls == 7); }
    calls = 0;
    try {
        disk::RetryDeadlockedTransaction([&]() -> int { ++calls; throw boost::system::system_error(boost::mysql::common_server_errc::er_lock_wait_timeout); });
        Check(false);
    } catch (const boost::system::system_error&) { Check(calls == 1); }
    calls = 0;
    try {
        disk::RetryDeadlockedTransaction([&]() -> int { ++calls; throw std::runtime_error("unknown commit outcome"); });
        Check(false);
    } catch (const std::runtime_error&) { Check(calls == 1); }
    std::cout << "Retry bounds, unwind and non-retryable error checks passed\n";
}
