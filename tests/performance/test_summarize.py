import csv
from pathlib import Path
import tempfile
import unittest

from summarize import aggregate, observations, resource_summary


class ReportAccuracy(unittest.TestCase):
    def test_repeats_preserve_failures_and_use_medians(self):
        rows = [dict(scenario='profile-v8', route='GET /api/profile', vus=8,
                     samples=1000, failures=failures, rps=rps, p50=rps / 10,
                     p95=rps / 5, p99=rps / 2, mib_s=None, dropped=0, exit_code=code)
                for rps, failures, code in [(100, 0, 0), (900, 7, 99), (110, 0, 0)]]
        result, = aggregate(rows)
        self.assertEqual(result['rps_median'], 110)
        self.assertEqual((result['rps_min'], result['rps_max']), (100, 900))
        self.assertEqual(result['samples'], 3000)
        self.assertEqual(result['failures'], 7)
        self.assertEqual(result['failed_trials'], 1)
        self.assertEqual(result['p99_median'], 55)

    def test_file_throughput_is_not_confused_with_rps(self):
        row = dict(scenario='upload', route='POST /api/files', vus=4,
                   samples=20, failures=0, rps=2, p50=10, p95=20, p99=30,
                   mib_s=64, dropped=0, exit_code=0)
        result, = aggregate([row])
        self.assertEqual(result['mib_s_median'], 64)
        self.assertEqual(result['rps_median'], 2)

    def test_failed_file_attempts_and_cleanup_are_retained(self):
        run = {'results': [dict(kind='file', label='upload-v4-r1', repeat=1,
            route='POST /api/files', vus=4, completed=8, attempted=10, errors=2, cleanup_errors=1,
            operations_per_second=4, payload_mib_per_second=4, p50_ms=10, p95_ms=20, p99_ms=30)]}
        result, = aggregate(observations(Path('.'), run))
        self.assertEqual(result['samples'], 10)
        self.assertEqual(result['failure_percent'], 20)
        self.assertEqual(result['cleanup_errors'], 1)

    def test_cpu_delta_does_not_cross_case_or_pid_boundaries(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'resources.csv'
            with path.open('w', newline='', encoding='utf-8') as stream:
                writer = csv.writer(stream)
                writer.writerow(['timestamp', 'case', 'process', 'pid', 'cpu_seconds', 'rss_bytes', 'private_bytes'])
                writer.writerows([
                    [0, 'setup', 'api', 7, 0, 1048576, 2097152],
                    [1, 'profile-r1', 'api', 7, 10, 2097152, 3145728],
                    [2, 'profile-r1', 'api', 7, 11, 3145728, 4194304],
                    [3, 'profile-r2', 'api', 7, 99, 2097152, 3145728],
                    [4, 'profile-r2', 'api', 7, 99.5, 2097152, 3145728],
                ])
            result, = resource_summary(path)
            self.assertEqual(result['mean_cpu_cores'], .75)
            self.assertEqual(result['peak_rss_mib'], 3)
            self.assertEqual(result['peak_private_mib'], 4)


if __name__ == '__main__':
    unittest.main()
