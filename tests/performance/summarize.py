"""Create a shareable baseline from run.py artifacts; never export fixtures or tokens."""
import argparse
import csv
import json
from pathlib import Path
import re
import statistics
import subprocess


def read_json(path):
    return json.loads(path.read_text(encoding='utf-8'))


def observations(directory, run):
    rows = []
    for item in run['results']:
        if item['kind'] == 'tcp':
            continue
        if item['kind'] == 'file':
            rows.append(dict(
                scenario=re.sub(r'-r\d+$', '', item['label']), route=item['route'],
                repeat=item['repeat'], vus=item['vus'], samples=item.get('attempted', item['completed']),
                failures=item.get('errors', 0), rps=item['operations_per_second'],
                p50=item['p50_ms'], p95=item['p95_ms'], p99=item['p99_ms'],
                mib_s=item['payload_mib_per_second'], dropped=0,
                cleanup_errors=item.get('cleanup_errors', 0), exit_code=0))
            continue
        data = read_json(directory / item['summary_file'])
        seconds = data['summary']['state']['testRunDurationMs'] / 1000
        dropped = data['summary']['metrics'].get('dropped_iterations', {}).get('values', {}).get('count', 0)
        for route, values in data['routes'].items():
            if not values['requests']:
                continue
            trend = values['duration_ms']
            rows.append(dict(
                scenario=re.sub(r'-r\d+$', '', item['label']), route=route,
                vus=item['vus'], samples=values['requests'],
                failures=values['requests'] - values['successes'],
                rps=values['successes'] / seconds, p50=trend['med'],
                p95=trend['p(95)'], p99=trend['p(99)'], mib_s=None,
                dropped=dropped, exit_code=item['exit_code']))
    return rows


def aggregate(rows):
    groups = {}
    for row in rows:
        groups.setdefault((row['scenario'], row['route']), []).append(row)
    result = []
    for (scenario, route), trials in groups.items():
        samples = sum(row['samples'] for row in trials)
        failures = sum(row['failures'] for row in trials)
        rates = [row['rps'] for row in trials]
        result.append(dict(
            scenario=scenario, route=route, vus=trials[0]['vus'], repeats=len(trials),
            samples=samples, minimum_samples=min(row['samples'] for row in trials),
            failures=failures, failure_percent=100 * failures / samples if samples else None,
            rps_median=statistics.median(rates), rps_min=min(rates), rps_max=max(rates),
            p50_median=statistics.median(row['p50'] for row in trials),
            p95_median=statistics.median(row['p95'] for row in trials),
            p99_median=statistics.median(row['p99'] for row in trials),
            mib_s_median=statistics.median(row['mib_s'] for row in trials) if trials[0]['mib_s'] is not None else None,
            dropped=sum(row['dropped'] for row in trials),
            cleanup_errors=sum(row.get('cleanup_errors', 0) for row in trials),
            failed_trials=sum(row['exit_code'] != 0 for row in trials)))
    return result


def resource_summary(path):
    if not path.exists():
        return []
    groups, previous = {}, {}
    with path.open(encoding='utf-8-sig', newline='') as stream:
        for row in csv.DictReader(stream):
            if row['case'].startswith('warmup') or row['case'] in ('setup', 'recovery'):
                previous.pop(row['pid'], None)
                continue
            key = (re.sub(r'-r\d+$', '', row['case']), row['process'])
            group = groups.setdefault(key, {'peak_rss_bytes': 0, 'peak_private_bytes': 0, 'cpu_seconds': 0, 'seconds': 0})
            group['peak_rss_bytes'] = max(group['peak_rss_bytes'], int(row['rss_bytes']))
            group['peak_private_bytes'] = max(group['peak_private_bytes'], int(row['private_bytes']))
            last = previous.get(row['pid'])
            if last and last['case'] == row['case']:
                elapsed = float(row['timestamp']) - float(last['timestamp'])
                cpu = float(row['cpu_seconds']) - float(last['cpu_seconds'])
                if elapsed > 0 and cpu >= 0:
                    group['seconds'] += elapsed
                    group['cpu_seconds'] += cpu
            previous[row['pid']] = row
    return [dict(scenario=key[0], process=key[1],
                 mean_cpu_cores=values['cpu_seconds'] / values['seconds'] if values['seconds'] else None,
                 peak_rss_mib=values['peak_rss_bytes'] / 1048576,
                 peak_private_mib=values['peak_private_bytes'] / 1048576)
            for key, values in groups.items() if key[1] in ('api', 'http-server')]


def render(directory, run, rows, resources):
    metadata = run['metadata']
    args = metadata['args']
    provenance = read_json(directory / 'provenance.json')
    invariants_path = directory / 'invariants.json'
    invariants = read_json(invariants_path) if invariants_path.exists() else None
    lines = [
        '# Disk / gin_cpp 性能基线', '',
        '本报告是指定环境下的实测基线。负载端和 API 共享 Windows 主机，存储节点位于同机 WSL，网络为回环链路。',
        '这些数据用于比较相同条件下的版本变化，不代表独立部署的容量上限。', '',
        '## 版本与环境', '',
        f"- Disk：`{provenance['disk']['commit']}`；工作树：{'有修改，见 provenance.json 和源码哈希' if provenance['disk']['status'].strip() else '干净'}。",
        f"- gin_cpp：`{provenance.get('gin_cpp', {}).get('commit', '未提供')}`。",
        f"- MouduAsio：`{provenance.get('MouduAsio', {}).get('commit', '未提供')}`；工作树{'有修改，以 network-sha256.json 记录的实际网络源码为准' if provenance.get('MouduAsio', {}).get('status', '').strip() else '干净'}。",
        f"- 系统：`{metadata['platform']}`；逻辑处理器：{metadata['logical_cpus']}。",
        f"- CPU：{metadata.get('cpu_model', metadata['processor'])}；物理内存：{metadata.get('physical_memory_bytes', 0) / 1073741824:.1f} GiB。",
        f"- API 构建：{metadata['configuration']}；存储构建：{metadata['storage_configuration']}。",
        f"- API 编译器：`{metadata['compiler']}`。",
        f"- gRPC 依赖：`{metadata['grpc_cmake_directory']}`；本机安装目录的静态库为 Debug 构建。",
        f"- 每项 HTTP/API 预热 {args['warmup']} 秒，正式负载 {args['duration']} 秒，重复 {args['repeats']} 轮。",
        '- gin_cpp 微基准不启用逐请求 Logger；真实 Disk API 保留其现有日志和业务中间件。',
        '- API、MySQL 与存储使用独立测试数据库和对象目录；数据库、缓存和日志不在重复轮次之间重置。', '',
        '## HTTP 与业务 API', '',
        'RPS 为成功目标请求数除以实际测试时长。表中 RPS、P50/P95/P99 均取各轮统计值的中位数；RPS 范围列出各轮最小值和最大值。',
        '分位数中位数不等于合并所有请求后的分位数。错误率包含非成功状态码和正文校验失败。', '',
        'Windows 计时可能将极短请求记录为 0 ms；该值表示计时分辨率下未测得正时长，不能解释为零延迟。', '',
        '| 场景 | 路由 | 配置 VU | 轮数 | 成功 RPS 中位数（范围） | P50 ms | P95 ms | P99 ms | 失败/请求 | 丢弃迭代 |',
        '|---|---|---:|---:|---:|---:|---:|---:|---:|---:|']
    for row in rows:
        if row['mib_s_median'] is not None:
            continue
        lines.append(f"| {row['scenario']} | {row['route']} | {row['vus']} | {row['repeats']} | "
                     f"{row['rps_median']:.2f}（{row['rps_min']:.2f}–{row['rps_max']:.2f}） | "
                     f"{row['p50_median']:.3f} | {row['p95_median']:.3f} | {row['p99_median']:.3f} | "
                     f"{row['failures']}/{row['samples']}（{row['failure_percent']:.2f}%） | {row['dropped']} |")
    lines += ['', '## 文件传输', '',
              '文件场景包含首次传输与客户端哈希校验，不单独预热。每次上传后立即软删除、查回收站并永久删除；这些清理请求占用容量，未计入上传延迟，但计入吞吐分母。',
              '下载逐次检查长度和完整 SHA-256。文件延迟分位数只统计成功操作；失败尝试及其耗时单独保存在原始 JSON。MiB/s 只计成功传输。', '',
              '| 场景 | 并发 | 轮数 | 成功操作/s | MiB/s | 成功 P95 ms | 成功 P99 ms | 失败/尝试 | 清理失败 |',
              '|---|---:|---:|---:|---:|---:|---:|---:|---:|']
    for row in rows:
        if row['mib_s_median'] is None:
            continue
        lines.append(f"| {row['scenario']} | {row['vus']} | {row['repeats']} | {row['rps_median']:.2f} | "
                     f"{row['mib_s_median']:.2f} | {row['p95_median']:.3f} | {row['p99_median']:.3f} | "
                     f"{row['failures']}/{row['samples']}（{row['failure_percent']:.2f}%） | {row['cleanup_errors']} |")
    lines += ['', '## 进程资源', '',
              '每秒采样；CPU 以平均占用核心数表示，1.0 表示占用一个逻辑核心。内存为测量轮次期间观察到的峰值，无法覆盖采样间隔内的瞬时尖峰。',
              '这里只列服务进程；客户端、MySQL 和 Linux 存储节点的原始采样另行保留。', '',
              '| 场景 | 进程 | 平均 CPU 核 | 峰值工作集 MiB | 峰值私有内存 MiB |',
              '|---|---|---:|---:|---:|']
    for row in resources:
        cpu = f"{row['mean_cpu_cores']:.3f}" if row['mean_cpu_cores'] is not None else '样本不足'
        lines.append(f"| {row['scenario']} | {row['process']} | {cpu} | {row['peak_rss_mib']:.2f} | {row['peak_private_mib']:.2f} |")
    lines += ['', '## 正确性与限制', '']
    if invariants is not None:
        lines += ['压测后的数据库与物理对象核验：', '', '```json', json.dumps(invariants, indent=2, ensure_ascii=False), '```', '']
    else:
        lines += ['本轮未执行数据库与对象一致性检查。', '']
    if (directory / 'integration-adaptation.json').exists():
        lines += ['业务回归执行原分支测试的副本：仅将 HTTP 头读取改为大小写不敏感，并将子进程日志持续写入日志文件，避免未消费的 PIPE 阻塞。',
                  '全部业务断言保留；原始与执行脚本哈希见 integration-adaptation.json。', '']
    failed = run.get('failure') or any(row['failed_trials'] or row['failures'] or row['dropped'] or row['cleanup_errors'] for row in rows)
    if run.get('failure'):
        lines += ['本轮中断原因：' + run['failure'], '']
    lines += [f"- 本轮状态：{'存在失败、丢弃迭代或未完成阶段，见原始结果；不可作为全成功基线。' if failed else '已完成；已记录场景未出现失败或丢弃迭代。'}",
              f"- 本轮正式负载每项 {args['duration']} 秒；容量评估应采用 60 秒以上轮次、到达速率阶梯与持续负载。",
              '- 并发 VU 表示负载端的并行虚拟用户，不代表服务端最大连接数。',
              '- rate 场景采用固定迭代到达速率，配置 VU 为预分配值，最大允许扩展到 200 VU；短连接场景固定为 200 次迭代/秒。',
              '- 文件场景的 Python 客户端、哈希和清理请求会限制负载发起能力。',
              '- 本轮不涵盖断网、进程崩溃、磁盘满或对象清理重试，也未据此证明长期无内存泄漏。']
    low_samples = [row['scenario'] for row in rows if row['minimum_samples'] < 1000]
    if low_samples:
        lines.append('- 以下场景有轮次少于 1000 个样本，P99 易波动：' + '、'.join(low_samples) + '。')
    lines += ['', '## 复现', '',
              '构建与运行步骤见 `tests/performance/README.md`。本轮原始命令如下；更换机器后替换源码、工具及构建目录。', '',
              '```text', subprocess.list2cmdline(metadata['command']), '```', '',
              '原始结果目录：`' + str(directory) + '`。',
              '保留 metadata.json、provenance.json、各场景 JSON、resources.csv、*sha256.json、构建与功能回归日志。',
              'fixtures.json 含测试账号与令牌，不应随公开报告发布。', '']
    return '\n'.join(lines)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    directory = args.directory.resolve()
    run = read_json(directory / 'results.json')
    rows = aggregate(observations(directory, run))
    resources = resource_summary(directory / 'resources.csv')
    output = args.output or directory / 'baseline-zh.md'
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(render(directory, run, rows, resources), encoding='utf-8')
    (directory / 'baseline-summary.json').write_text(json.dumps({'scenarios': rows, 'resources': resources}, indent=2), encoding='utf-8')
    print(output)


if __name__ == '__main__':
    main()
