"""Build-independent performance runner. All generated data stays under build/performance."""
import argparse
import concurrent.futures
import contextlib
import csv
import ctypes
from ctypes import wintypes
import hashlib
import http.client
import json
import math
import os
from pathlib import Path
import platform
import shlex
import socket
import subprocess
import sys
import threading
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
CREATE_FLAGS = subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0


def save(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False), encoding='utf-8')


def free_port():
    with socket.socket() as stream:
        stream.bind(('127.0.0.1', 0))
        return stream.getsockname()[1]


def execute(command, **kwargs):
    return subprocess.run([str(item) for item in command], creationflags=CREATE_FLAGS,
                          check=True, **kwargs)


def request(port, method, path, body=None, token=None, headers=None, expected=200, connection=None):
    outgoing = dict(headers or {})
    if token:
        outgoing['Authorization'] = 'Bearer ' + token
    if isinstance(body, dict):
        outgoing['Content-Type'] = 'application/json'
        body = json.dumps(body).encode()
    client = connection or http.client.HTTPConnection('127.0.0.1', port, timeout=60)
    try:
        client.request(method, path, body, outgoing)
        response = client.getresponse()
        data = response.read()
        if response.status != expected:
            raise RuntimeError(f'{method} {path}: HTTP {response.status}, {data[:250]!r}')
        return data
    finally:
        if connection is None:
            client.close()


def json_request(*args, **kwargs):
    return json.loads(request(*args, **kwargs))


def upload(port, token, folder, name, data):
    boundary = 'perf-' + uuid.uuid4().hex
    prefix = (f'--{boundary}\r\nContent-Disposition: form-data; name="folder_id"\r\n\r\n{folder}\r\n'
              f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{name}"\r\n'
              'Content-Type: application/octet-stream\r\n\r\n').encode()
    body = prefix + data + f'\r\n--{boundary}--\r\n'.encode()
    return json_request(port, 'POST', '/api/files', body, token,
                        {'Content-Type': f'multipart/form-data; boundary={boundary}',
                         'X-Upload-Size': str(len(data))})


class Resources:
    """Sample CPU time and working/private bytes without third-party Python packages."""
    def __init__(self, directory):
        self.processes = {}
        self.processes[os.getpid()] = (None, 'runner')
        self.lock = threading.Lock()
        self.stop = threading.Event()
        self.directory = directory
        self.label = 'setup'
        if os.name == 'nt':
            self.kernel = ctypes.WinDLL('kernel32', use_last_error=True)
            self.psapi = ctypes.WinDLL('psapi', use_last_error=True)
            self.kernel.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
            self.kernel.OpenProcess.restype = wintypes.HANDLE
            self.kernel.CloseHandle.argtypes = [wintypes.HANDLE]
            self.kernel.GetProcessTimes.argtypes = [wintypes.HANDLE] + [ctypes.POINTER(wintypes.FILETIME)] * 4
            self.psapi.GetProcessMemoryInfo.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD]
        self.thread = threading.Thread(target=self.run, daemon=True)
        self.thread.start()

    def add(self, process, name):
        with self.lock:
            self.processes[process.pid] = (process, name)

    def add_pid(self, pid, name):
        with self.lock:
            self.processes[pid] = (None, name)

    def read(self, pid):
        if os.name != 'nt':
            return None
        class Memory(ctypes.Structure):
            _fields_ = [('cb', wintypes.DWORD), ('faults', wintypes.DWORD)] + [
                (field, ctypes.c_size_t) for field in
                ('peak_rss', 'rss', 'peak_paged', 'paged', 'peak_nonpaged', 'nonpaged', 'pagefile', 'peak_pagefile', 'private')]
        handle = self.kernel.OpenProcess(0x1000 | 0x10, False, pid)
        if not handle:
            return None
        try:
            times = [wintypes.FILETIME() for _ in range(4)]
            memory = Memory()
            memory.cb = ctypes.sizeof(memory)
            if not self.kernel.GetProcessTimes(handle, *[ctypes.byref(value) for value in times]):
                return None
            if not self.psapi.GetProcessMemoryInfo(handle, ctypes.byref(memory), memory.cb):
                return None
            cpu = sum((value.dwHighDateTime << 32) + value.dwLowDateTime for value in times[2:]) / 1e7
            return cpu, memory.rss, memory.private
        finally:
            self.kernel.CloseHandle(handle)

    def run(self):
        with (self.directory / 'resources.csv').open('w', newline='', encoding='utf-8') as stream:
            writer = csv.writer(stream)
            writer.writerow(['timestamp', 'case', 'process', 'pid', 'cpu_seconds', 'rss_bytes', 'private_bytes'])
            while not self.stop.is_set():
                with self.lock:
                    processes = list(self.processes.items())
                for pid, (process, name) in processes:
                    if process is not None and process.poll() is not None:
                        continue
                    values = self.read(pid)
                    if values:
                        sample = [time.time(), self.label, name, pid, *values]
                        writer.writerow(sample)
                stream.flush()
                self.stop.wait(1)

    def close(self):
        self.stop.set()
        self.thread.join()


@contextlib.contextmanager
def managed_process(command, name, directory, resources, environment=None):
    with (directory / f'{name}.stdout.log').open('w', encoding='utf-8') as stdout, \
         (directory / f'{name}.stderr.log').open('w', encoding='utf-8') as stderr:
        process = subprocess.Popen([str(item) for item in command], env=environment,
                                   stdout=stdout, stderr=stderr, creationflags=CREATE_FLAGS)
        resources.add(process, name)
        try:
            yield process
        finally:
            stopped_by_runner = process.poll() is None
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=15)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=15)
            save(directory / f'{name}.process.json', {'command': [str(item) for item in command],
                 'exit_code': process.returncode, 'stopped_by_runner': stopped_by_runner})


def wait_until(check, process, name, timeout=60):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f'{name} exited with {process.returncode}; inspect its stdout/stderr logs')
        try:
            if check():
                return
        except (OSError, RuntimeError, json.JSONDecodeError):
            pass
        time.sleep(0.2)
    raise RuntimeError(f'{name} did not become ready within {timeout}s')


def linux_path(path):
    path = Path(path).resolve()
    return '/mnt/' + path.drive[0].lower() + path.as_posix()[2:]


@contextlib.contextmanager
def local_api(args, directory, resources):
    runtime = ROOT / 'build/performance/runtime'
    mysql_data = runtime / 'mysql'
    runtime.mkdir(parents=True, exist_ok=True)
    mysql_bin = Path(args.mysql_bin)
    mysql = mysql_bin / 'mysql.exe'
    mysqld = mysql_bin / 'mysqld.exe'
    if not mysqld.exists():
        raise RuntimeError('Specify --mysql-bin for a local MySQL installation')
    if not mysql_data.exists():
        mysql_data.mkdir()
        with (directory / 'mysql-initialize.log').open('w', encoding='utf-8') as log:
            execute([mysqld, '--no-defaults', '--initialize-insecure', f'--basedir={mysql_bin.parent}',
                     f'--datadir={mysql_data}', f'--log-error={runtime / "mysql.log"}'],
                    stdout=log, stderr=subprocess.STDOUT, timeout=180)
    db_port, storage_port, api_port = free_port(), free_port(), free_port()
    schema = 'disk_perf_' + uuid.uuid4().hex[:12]
    mysql_command = [mysql, '--no-defaults', '--protocol=TCP', '--host=127.0.0.1',
                     f'--port={db_port}', '--user=root', '--raw', '--batch', '--skip-column-names']

    def sql(statement):
        return execute(mysql_command, input=statement, text=True, capture_output=True, timeout=60).stdout.strip()

    with contextlib.ExitStack() as stack:
        database = stack.enter_context(managed_process([
            mysqld, '--no-defaults', f'--basedir={mysql_bin.parent}', f'--datadir={mysql_data}',
            '--bind-address=127.0.0.1', f'--port={db_port}', '--mysqlx=OFF',
            f'--log-error={runtime / "mysql.log"}', '--skip-log-bin',
        ], 'mysql', directory, resources))
        def database_ready():
            try:
                return sql('SELECT 1') == '1'
            except subprocess.CalledProcessError:
                return False
        wait_until(database_ready, database, 'MySQL')
        mysql_worker_pid = int(Path(sql('SELECT @@pid_file')).read_text(encoding='utf-8').strip())
        if mysql_worker_pid != database.pid:
            resources.add_pid(mysql_worker_pid, 'mysql-worker')
        stack.callback(lambda: subprocess.run(mysql_command[:-2] + ['--execute=SHUTDOWN'],
                                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                                              creationflags=CREATE_FLAGS, timeout=30))
        sql(f'CREATE DATABASE {schema} CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci')
        mysql_command.append('--database=' + schema)
        for migration in sorted((args.source / 'migrations').glob('*.up.sql')):
            sql(migration.read_text(encoding='utf-8'))
        objects = runtime / schema / 'objects'
        objects.mkdir(parents=True)
        storage_binary = args.storage_build / 'grpc_storage_node'
        if not storage_binary.exists():
            raise RuntimeError('Build the Linux storage node into build/storage-linux first')
        launch_script = directory / 'storage-launch.sh'
        launch_script.write_text('echo STORAGE_PID=$$\nexec ' + shlex.join([
            linux_path(storage_binary), f'127.0.0.1:{storage_port}', linux_path(objects)]) + '\n', encoding='utf-8', newline='\n')
        storage = stack.enter_context(managed_process([
            'wsl', '-d', args.distro, '--exec', 'sh', linux_path(launch_script),
        ], 'storage', directory, resources))
        def stop_storage():
            lines = (directory / 'storage.stdout.log').read_text(encoding='utf-8').splitlines()
            pid_line = next((line for line in lines if line.startswith('STORAGE_PID=')), '')
            if pid_line:
                pid = int(pid_line.split('=')[1])
                subprocess.run(['wsl', '-d', args.distro, '--', 'kill', '-TERM', str(pid)],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                               creationflags=CREATE_FLAGS, timeout=20)
        stack.callback(stop_storage)
        wait_until(lambda: 'gRPC storage listening' in
                   (directory / 'storage.stdout.log').read_text(encoding='utf-8'), storage, 'Storage', timeout=90)
        def storage_reachable():
            with socket.create_connection(('127.0.0.1', storage_port), timeout=1):
                return True
        wait_until(storage_reachable, storage, 'Storage loopback forwarding', timeout=30)
        storage_pid = int(next(line for line in (directory / 'storage.stdout.log').read_text(encoding='utf-8').splitlines()
                               if line.startswith('STORAGE_PID=')).split('=')[1])
        monitor_script = directory / 'storage-monitor.sh'
        monitor_script.write_text(
            'echo "clock_ticks=$(getconf CLK_TCK),page_bytes=$(getconf PAGESIZE)"\n'
            'echo timestamp,cpu_ticks,rss_pages,threads\n'
            f'while test -r /proc/{storage_pid}/stat; do\n'
            f'  awk -v stamp="$(date +%s)" \'{{print stamp "," $14+$15 "," $24 "," $20}}\' /proc/{storage_pid}/stat\n'
            '  sleep 1\ndone\n', encoding='utf-8', newline='\n')
        stack.enter_context(managed_process(['wsl', '-d', args.distro, '--exec', 'sh', linux_path(monitor_script)],
                                            'storage-resources', directory, resources))
        environment = os.environ.copy()
        environment.update(DB_HOST='127.0.0.1', DB_PORT=str(db_port), DB_NAME=schema,
                           DB_USER='root', DB_PASSWORD='', PORT=str(api_port), LISTEN='127.0.0.1',
                           IO_THREADS='2', JWT_ISSUER='disk', JWT_SECRET=uuid.uuid4().hex + uuid.uuid4().hex,
                           STORAGE_ADDRESS=f'127.0.0.1:{storage_port}', MYSQL_CLIENT=str(mysql),
                           STORAGE_TEST_ROOT=str(objects), UPLOAD_CONCURRENT_GLOBAL='4',
                           UPLOAD_IDLE_TIMEOUT_SECONDS='30', REQUEST_TIMEOUT_SECONDS='1800')
        for key in ('STORAGE_TLS_CA', 'STORAGE_TLS_CLIENT_CERT', 'STORAGE_TLS_CLIENT_KEY', 'STORAGE_TLS_SERVER_NAME'):
            environment.pop(key, None)
        save(directory / 'environment.json', {
            'db_host': '127.0.0.1', 'db_port': db_port, 'db_name': schema,
            'storage_port': storage_port, 'api_port': api_port, 'objects': str(objects),
            'io_threads': 2, 'business_threads': 4, 'mysql_pool_max': 8,
            'upload_global': 4, 'upload_per_user': 2,
        })
        if not args.skip_integration:
            print('Running real MySQL + gRPC API regression', flush=True)
            with (directory / 'api-integration.log').open('w', encoding='utf-8') as log:
                execute([sys.executable, HERE / 'api_regression.py', args.source, args.build / 'disk_api.exe', directory],
                        env=environment, stdout=log, stderr=subprocess.STDOUT, timeout=240)
        api = stack.enter_context(managed_process([args.build / 'disk_api.exe'], 'api', directory,
                                                  resources, environment))
        wait_until(lambda: json_request(api_port, 'GET', '/readyz'), api, 'API')
        try:
            yield api_port, sql, environment
        finally:
            try:
                (directory / 'innodb-status.txt').write_text(sql('SHOW ENGINE INNODB STATUS'), encoding='utf-8')
                (directory / 'mysql-status.txt').write_text(sql("SHOW GLOBAL STATUS WHERE Variable_name IN "
                    "('Innodb_deadlocks','Threads_connected','Threads_running','Slow_queries')"), encoding='utf-8')
                (directory / 'mysql-deadlocks.txt').write_text(sql("SELECT NAME,COUNT FROM information_schema.INNODB_METRICS "
                    "WHERE NAME='lock_deadlocks'"), encoding='utf-8')
            except subprocess.SubprocessError:
                pass


def prepare_fixtures(port, directory, accounts, dataset):
    password = 'performance-' + uuid.uuid4().hex
    fixtures = {'password': password, 'accounts': []}
    for index in range(accounts):
        username = 'perf_' + uuid.uuid4().hex[:16]
        token = json_request(port, 'POST', '/register', {'username': username, 'password': password})['access_token']
        root = json_request(port, 'GET', '/api/folders/root/children', token=token)['root']['id']
        folder = json_request(port, 'POST', '/api/folders', {'parent_id': root, 'name': 'performance'}, token)['id']
        for item in range(dataset):
            json_request(port, 'POST', '/api/folders', {'parent_id': folder, 'name': f'seed-{item:06d}'}, token)
        content = b'performance-download-seed\x00\xff' * 1024
        file = upload(port, token, folder, 'seed-file.bin', content)
        fixtures['accounts'].append({'username': username, 'token': token, 'token_at': int(time.time() * 1000),
                                     'root': root, 'folder': folder, 'file': file['id'],
                                     'sha256': file['sha256'], 'size': len(content)})
        print(f'Prepared account {index + 1}/{accounts}, {dataset} folders', flush=True)
    save(directory / 'fixtures.json', fixtures)
    return fixtures


def k6_run(args, directory, resources, port, case, vus, label, seconds=None, extra=None):
    seconds = seconds or args.duration
    fixture_path = directory / 'fixtures.json'
    public_cases = {'plaintext', 'json', 'echo', 'login', 'register', 'livez', 'readyz'}
    if fixture_path.exists() and case not in public_cases:
        fixtures = json.loads(fixture_path.read_text(encoding='utf-8'))
        if any(time.time() * 1000 - account.get('token_at', 0) > 540000 for account in fixtures['accounts']):
            resources.label = label + '-token-preparation'
            for account in fixtures['accounts']:
                account['token'] = json_request(port, 'POST', '/login', {
                    'username': account['username'], 'password': fixtures['password']})['access_token']
                account['token_at'] = int(time.time() * 1000)
            save(fixture_path, fixtures)
    environment = os.environ.copy()
    environment.update(BASE_URL=f'http://127.0.0.1:{port}', CASE=case, VUS=str(vus),
                       DURATION=f'{seconds}s', RUN_ID=uuid.uuid4().hex[:10],
                       SUMMARY_FILE=str(directory / f'{label}.json'))
    if (directory / 'fixtures.json').exists():
        environment['FIXTURES'] = str(directory / 'fixtures.json')
    environment.update(extra or {})
    resources.label = label
    print(f'{label}: {seconds}s', flush=True)
    command = [args.k6, 'run', '--quiet', HERE / 'routes.js']
    started = time.monotonic()
    with managed_process(command, label, directory, resources, environment) as process:
        code = process.wait(timeout=seconds + 120)
    if code not in (0, 99):
        raise RuntimeError(f'{label} failed with {code}; inspect its JSON and stderr log')
    result = {'label': label, 'kind': 'http', 'case': case, 'vus': vus, 'seconds': seconds,
              'exit_code': code, 'wall_seconds': time.monotonic() - started, 'summary_file': f'{label}.json'}
    if code:
        failure = RuntimeError(f'{label}: k6 response validation or threshold failed (exit {code})')
        failure.result = result
        raise failure
    return result


def tcp_case(args, directory, resources, port, label, server_threads, connections, size, extra=None):
    resources.label = label
    print(label, flush=True)
    command = [args.build / 'disk_tcp_benchmark.exe', 'client', '--port', port,
               '--connections', connections, '--bytes', size, '--threads', args.client_threads,
               '--warmup', args.warmup, '--duration', args.duration] + (extra or [])
    with managed_process(command, label, directory, resources) as client:
        code = client.wait(timeout=args.duration + args.warmup + 30)
    value = json.loads((directory / f'{label}.stdout.log').read_text(encoding='utf-8'))
    if value['invalid']:
        raise RuntimeError(f'{label} returned corrupted echo data')
    value.update(label=label, kind='tcp', server_threads=server_threads, exit_code=code)
    save(directory / f'{label}.json', value)
    return value


def run_tcp(args, directory, resources, results):
    for server_threads in args.io_threads:
        name = f'tcp-server-{server_threads}'
        port = free_port()
        with managed_process([args.build / 'disk_tcp_benchmark.exe', 'server', '--port', port,
                              '--threads', server_threads, '--duration', 86400], name, directory, resources) as server:
            wait_until(lambda: 'READY' in (directory / f'{name}.stdout.log').read_text(encoding='utf-8'),
                       server, name)
            for size in args.message_bytes:
                for connections in args.connections:
                    for repeat in range(args.repeats):
                        label = f'tcp-t{server_threads}-b{size}-c{connections}-r{repeat + 1}'
                        results.append(tcp_case(args, directory, resources, port, label, server_threads, connections, size))
            if args.tcp_extras:
                results.append(tcp_case(args, directory, resources, port, f'tcp-t{server_threads}-reconnect',
                                        server_threads, 10, 64, ['--reconnect', 1, '--reconnect-delay-ms', 20]))
                results.append(tcp_case(args, directory, resources, port, f'tcp-t{server_threads}-slow-read',
                                        server_threads, 10, 65536, ['--read-delay-ms', 10]))
                results.append(tcp_case(args, directory, resources, port, f'tcp-t{server_threads}-c1000',
                                        server_threads, 1000, 1024))


def run_http(args, directory, resources, results):
    port = free_port()
    with managed_process([args.http_build / 'disk_http_benchmark.exe', '--port', port, '--threads', args.http_threads,
                          '--duration', 86400], 'http-server', directory, resources) as server:
        wait_until(lambda: request(port, 'GET', '/benchmark/plaintext') == b'Hello, World!', server, 'HTTP')
        for case in ('plaintext', 'json', 'echo'):
            for connections in args.connections:
                if args.warmup:
                    k6_run(args, directory, resources, port, case, connections,
                           f'warmup-http-{case}-c{connections}', args.warmup)
                for repeat in range(args.repeats):
                    results.append(k6_run(args, directory, resources, port, case, connections,
                                          f'http-{case}-c{connections}-r{repeat + 1}'))
        results.append(k6_run(args, directory, resources, port, 'plaintext', args.short_vus,
                              'http-plaintext-short', extra={'SHORT_CONNECTIONS': '1', 'RATE': '200'}))


def payload_file(directory, size):
    path = directory / f'payload-{size}.bin'
    block = bytes(range(256)) * 256
    with path.open('wb') as stream:
        remaining = size
        while remaining:
            part = block[:min(len(block), remaining)]
            stream.write(part)
            remaining -= len(part)
    return path


def stream_upload(client, token, folder, path, marker):
    size = path.stat().st_size
    boundary = 'stream-' + uuid.uuid4().hex
    prefix = (f'--{boundary}\r\nContent-Disposition: form-data; name="folder_id"\r\n\r\n{folder}\r\n'
              f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="perf-{uuid.uuid4().hex}.bin"\r\n'
              'Content-Type: application/octet-stream\r\n\r\n').encode()
    suffix = f'\r\n--{boundary}--\r\n'.encode()
    digest = hashlib.sha256()
    started = time.perf_counter()
    client.putrequest('POST', '/api/files')
    client.putheader('Authorization', 'Bearer ' + token)
    client.putheader('Content-Type', 'multipart/form-data; boundary=' + boundary)
    client.putheader('Content-Length', str(len(prefix) + size + len(suffix)))
    client.putheader('X-Upload-Size', str(size))
    client.endheaders()
    client.send(prefix)
    with path.open('rb') as source:
        first = True
        while block := source.read(65536):
            if first and marker:
                count = min(len(marker), len(block))
                block = marker[:count] + block[count:]
            first = False
            digest.update(block)
            client.send(block)
    client.send(suffix)
    response = client.getresponse()
    ttfb = (time.perf_counter() - started) * 1000
    body = response.read()
    elapsed = (time.perf_counter() - started) * 1000
    if response.status != 200:
        raise RuntimeError(f'Stream upload HTTP {response.status}: {body[:250]!r}')
    value = json.loads(body)
    if value['size'] != size or value['sha256'] != digest.hexdigest():
        raise RuntimeError('Stream upload size/hash mismatch')
    return value, elapsed, ttfb


def file_load(args, directory, resources, port, fixtures, path, operation, vus, mode='duplicate', repeat=1):
    size = path.stat().st_size
    label = f'{operation}-{mode}-b{size}-v{vus}-r{repeat}'
    resources.label = label
    print(f'{label}: {args.duration}s', flush=True)
    tokens = [json_request(port, 'POST', '/login', {
        'username': fixtures['accounts'][index % len(fixtures['accounts'])]['username'],
        'password': fixtures['password']})['access_token'] for index in range(vus)]
    begin = time.perf_counter()
    deadline = begin + args.duration

    def worker(index):
        account = fixtures['accounts'][index % len(fixtures['accounts'])]
        token = tokens[index]
        authenticated = time.monotonic()
        samples = []
        errors, cleanup_errors = [], []
        client = http.client.HTTPConnection('127.0.0.1', port, timeout=120)
        try:
            while time.perf_counter() < deadline:
                if time.monotonic() - authenticated > 600:
                    token = json_request(port, 'POST', '/login', {
                        'username': account['username'], 'password': fixtures['password']})['access_token']
                    authenticated = time.monotonic()
                target_finished = False
                started = time.perf_counter()
                try:
                    if operation == 'upload':
                        marker = uuid.uuid4().hex.encode() if mode == 'unique' else b''
                        file, elapsed, ttfb = stream_upload(client, token, account['folder'], path, marker)
                        samples.append((elapsed, ttfb))
                        target_finished = True
                        request(port, 'DELETE', f'/api/files/{file["id"]}', token=token, expected=204, connection=client)
                        entries = json_request(port, 'GET', '/api/trash?page_size=100', token=token, connection=client)['items']
                        entry = next(item for item in entries if item['type'] == 'file' and item['resource_id'] == file['id'])
                        request(port, 'DELETE', f'/api/trash/{entry["id"]}', token=token, expected=204, connection=client)
                    else:
                        client.request('GET', f'/api/files/{account["file"]}/download',
                                       headers={'Authorization': 'Bearer ' + token})
                        response = client.getresponse()
                        ttfb = (time.perf_counter() - started) * 1000
                        if response.status != 200:
                            raise RuntimeError(f'Stream download HTTP {response.status}')
                        digest = hashlib.sha256()
                        received = 0
                        while block := response.read(65536):
                            received += len(block)
                            digest.update(block)
                        elapsed = (time.perf_counter() - started) * 1000
                        if received != size or digest.hexdigest() != account['sha256']:
                            raise RuntimeError('Stream download size/hash mismatch')
                        samples.append((elapsed, ttfb))
                except (OSError, RuntimeError, http.client.HTTPException, StopIteration, json.JSONDecodeError) as error:
                    # Record the failed attempt; do not silently retry it or count it as a success.
                    destination = cleanup_errors if target_finished else errors
                    destination.append({'error': str(error), 'elapsed_ms': (time.perf_counter() - started) * 1000})
                    client.close()
                    client = http.client.HTTPConnection('127.0.0.1', port, timeout=120)
        finally:
            client.close()
        return samples, errors, cleanup_errors

    with concurrent.futures.ThreadPoolExecutor(max_workers=vus) as pool:
        groups = list(pool.map(worker, range(vus)))
        samples = [sample for group, _, _ in groups for sample in group]
        errors = [error for _, group, _ in groups for error in group]
        cleanup_errors = [error for _, _, group in groups for error in group]
    elapsed = time.perf_counter() - begin
    latencies = sorted(sample[0] for sample in samples)
    if not samples and not errors:
        raise RuntimeError('File load produced no attempts')
    route = 'POST /api/files' if operation == 'upload' else 'GET /api/files/:id/download'
    value = {'label': label, 'kind': 'file', 'route': route, 'payload_bytes': size,
             'vus': vus, 'mode': mode, 'repeat': repeat, 'completed': len(samples), 'wall_seconds': elapsed,
             'attempted': len(samples) + len(errors), 'errors': len(errors), 'cleanup_errors': len(cleanup_errors),
             'failure_details': errors, 'cleanup_failure_details': cleanup_errors,
             'operations_per_second': len(samples) / elapsed,
             'payload_mib_per_second': len(samples) * size / elapsed / 1048576,
             'p50_ms': latencies[math.ceil(len(samples) * .5) - 1] if samples else 0,
             'p95_ms': latencies[math.ceil(len(samples) * .95) - 1] if samples else 0,
             'p99_ms': latencies[math.ceil(len(samples) * .99) - 1] if samples else 0,
             'ttfb_ms': [sample[1] for sample in samples], 'latency_ms': latencies}
    save(directory / f'{label}.json', value)
    return value


def health_stall(args, directory, resources, port):
    resources.label = 'health-storage-stall'
    pid = int(next(line for line in (directory / 'storage.stdout.log').read_text(encoding='utf-8').splitlines()
                   if line.startswith('STORAGE_PID=')).split('=')[1])

    def timed(path):
        client = http.client.HTTPConnection('127.0.0.1', port, timeout=15)
        begin = time.perf_counter()
        try:
            client.request('GET', path)
            response = client.getresponse()
            body = response.read()
            return {'path': path, 'status': response.status, 'milliseconds': (time.perf_counter() - begin) * 1000,
                    'body': body[:200].decode('utf-8', errors='replace')}
        finally:
            client.close()

    baseline = timed('/livez')
    execute(['wsl', '-d', args.distro, '--exec', 'kill', '-STOP', pid], capture_output=True)
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
            ready = pool.submit(timed, '/readyz')
            time.sleep(.2)
            live = [pool.submit(timed, '/livez') for _ in range(4)]
            time.sleep(.25)
            overflow = timed('/livez')
            observations = [ready.result()] + [result.result() for result in live]
    finally:
        execute(['wsl', '-d', args.distro, '--exec', 'kill', '-CONT', pid], capture_output=True)
    save(directory / 'health-storage-stall.json', {'baseline': baseline, 'paused_storage': observations,
                                                  'queue_overflow': overflow})
    live_statuses = [result['status'] for result in observations[1:]] + [overflow['status']]
    if observations[0]['status'] < 500 or not all(status in (200, 503) for status in live_statuses) or 503 not in live_statuses:
        raise RuntimeError('Unexpected health response during storage stall')
    for _ in range(10):
        try:
            request(port, 'GET', '/readyz')
            return
        except RuntimeError:
            time.sleep(.5)
    raise RuntimeError('Storage did not recover after SIGCONT')


def run_api(args, directory, resources, results):
    with local_api(args, directory, resources) as (port, sql, environment):
        fixtures = prepare_fixtures(port, directory, args.accounts, args.dataset_size)
        if 'api' in args.stages:
            for case in args.cases:
                if case == 'workflow':
                    continue
                for vus in args.api_vus:
                    if args.warmup:
                        k6_run(args, directory, resources, port, case, vus, f'warmup-api-{case}-v{vus}', args.warmup)
                    for repeat in range(args.repeats):
                        results.append(k6_run(args, directory, resources, port, case, vus, f'api-{case}-v{vus}-r{repeat + 1}'))
            if 'workflow' in args.cases:
                results.append(k6_run(args, directory, resources, port, 'workflow', 1, 'api-workflow-single'))
                results.append(k6_run(args, directory, resources, port, 'workflow', min(4, len(fixtures['accounts']) * 2),
                                      'api-workflow-multiple'))
            if 'profile' in args.cases:
                for rate in args.rates:
                    results.append(k6_run(args, directory, resources, port, 'profile', args.rate_vus or args.api_vus[-1],
                                          f'api-profile-rate{rate}', extra={'RATE': str(rate)}))
        if 'files' in args.stages:
            for size in args.file_bytes:
                path = payload_file(directory, size)
                for mode in ('duplicate', 'unique'):
                    for vus in sorted({1, min(4, len(fixtures['accounts']) * 2)}):
                        for repeat in range(args.repeats):
                            results.append(file_load(args, directory, resources, port, fixtures, path, 'upload', vus,
                                                     mode, repeat + 1))
                # Download through the complete HTTP -> gRPC -> disk path, with per-response SHA-256 checks.
                for account in fixtures['accounts']:
                    account['token'] = json_request(port, 'POST', '/login',
                                                    {'username': account['username'], 'password': fixtures['password']})['access_token']
                    account['token_at'] = int(time.time() * 1000)
                    client = http.client.HTTPConnection('127.0.0.1', port, timeout=120)
                    try:
                        file, _, _ = stream_upload(client, account['token'], account['folder'], path, b'')
                    finally:
                        client.close()
                    account.update(file=file['id'], sha256=file['sha256'], size=size)
                save(directory / 'fixtures.json', fixtures)
                for vus in (1, 4):
                    for repeat in range(args.repeats):
                        results.append(file_load(args, directory, resources, port, fixtures, path, 'download', vus,
                                                 repeat=repeat + 1))
        if 'mixed' in args.stages or 'soak' in args.stages:
            extra = {'ALLOW_429': '1', 'UPLOAD_FILE': str(payload_file(directory, 1048576))}
            if 'mixed' in args.stages:
                results.append(k6_run(args, directory, resources, port, 'mixed', args.api_vus[-1], 'mixed', extra=extra))
                results.append(k6_run(args, directory, resources, port, 'login_mixed', args.api_vus[-1], 'login-mixed'))
                health_stall(args, directory, resources, port)
            if 'soak' in args.stages:
                results.append(k6_run(args, directory, resources, port, 'mixed', args.api_vus[-1],
                                      'soak', args.soak_seconds, extra))
        resources.label = 'recovery'
        time.sleep(args.recovery)
        # Logical quota includes live and trashed files. Physical references include historical versions.
        invariant_sql = '''SELECT COUNT(*) FROM users u WHERE used_quota < 0 OR used_quota > quota_limit OR
            used_quota <> COALESCE((SELECT SUM(p.size) FROM files f JOIN physical_files p ON p.id=f.physical_file_id WHERE f.user_id=u.id),0);
            SELECT COUNT(*) FROM physical_files p WHERE NOT EXISTS(SELECT 1 FROM files f WHERE f.physical_file_id=p.id)
            AND NOT EXISTS(SELECT 1 FROM file_versions v WHERE v.physical_file_id=p.id);'''
        invariants = sql(invariant_sql).splitlines()
        referenced = set(sql('SELECT storage_key FROM physical_files').splitlines())
        objects = Path(environment['STORAGE_TEST_ROOT'])
        actual = {path.relative_to(objects).as_posix() for path in objects.rglob('*') if path.is_file()}
        missing, orphaned = sorted(referenced - actual), sorted(actual - referenced)
        save(directory / 'invariants.json', {'quota_mismatches': int(invariants[0]),
             'unreferenced_physical_records': int(invariants[1]), 'missing_objects': missing, 'orphaned_objects': orphaned})
        if any(int(value) for value in invariants) or missing or orphaned:
            raise RuntimeError('Post-load database invariants failed')


def report(directory, metadata, results, failure=None):
    save(directory / 'results.json', {'metadata': metadata, 'results': results, 'failure': failure})
    rows = []
    for result in results:
        if result['kind'] in ('tcp', 'file'):
            route = 'TCP echo' if result['kind'] == 'tcp' else result['route']
            rate = result.get('messages_per_second', result.get('operations_per_second'))
            rows.append([result['label'], route, result['completed'], rate,
                         result['p50_ms'], result['p95_ms'], result['p99_ms'], result.get('errors', 0) + result.get('invalid', 0)])
            continue
        value = json.loads((directory / result['summary_file']).read_text(encoding='utf-8'))
        seconds = value['summary']['state']['testRunDurationMs'] / 1000
        for route, metrics in value['routes'].items():
            trend = metrics['duration_ms']
            rows.append([result['label'], route, metrics['requests'], metrics['successes'] / seconds,
                         trend.get('med', 0), trend.get('p(95)', 0), trend.get('p(99)', 0),
                         metrics['requests'] - metrics['successes']])
    with (directory / 'routes.csv').open('w', newline='', encoding='utf-8-sig') as stream:
        writer = csv.writer(stream)
        writer.writerow(['case', 'route', 'requests', 'successes_per_second', 'p50_ms', 'p95_ms', 'p99_ms', 'non_successes'])
        writer.writerows(rows)
    lines = ['# Performance run', '', f"Commit `{metadata['commit']}`", '',
             f"Host `{metadata['platform']}`, {metadata['logical_cpus']} logical CPUs", '',
             'The load generator and tested services share the Windows host. API file storage runs in WSL.',
             f"Build configuration `{metadata['configuration']}`, gRPC CMake directory `{metadata['grpc_cmake_directory']}`.", '',
             'TCP RTT uses complete, validated echoes and 0.1 ms histogram buckets. TCP throughput is receive-side payload throughput.',
             'HTTP duration includes sending, waiting and receiving. Complete duration also includes the blocked/connection interval.',
             'Upload runs include cleanup traffic, which is excluded from target metrics and consumes server capacity.',
             'Linux storage CPU ticks, resident pages and thread counts are in storage-resources.stdout.log.', '',
             '| Case | Route | Samples | Successful operations/s | P95 ms | P99 ms | Non-successes |',
             '|---|---|---:|---:|---:|---:|---:|']
    for row in rows:
        lines.append(f'| {row[0]} | {row[1]} | {row[2]} | {row[3]:.2f} | {row[5]:.3f} | {row[6]:.3f} | {row[7]} |')
    if failure:
        lines.extend(['', '## Incomplete run', '', failure])
    threshold_failures = [result['label'] for result in results if result.get('exit_code') == 99]
    if threshold_failures:
        lines.extend(['', '## Failed thresholds', '', '\n'.join('- ' + label for label in threshold_failures)])
    (directory / 'report.md').write_text('\n'.join(lines) + '\n', encoding='utf-8')


def numbers(text):
    values = [int(item) for item in text.split(',')]
    if not values or any(value < 0 for value in values):
        raise argparse.ArgumentTypeError('Use comma-separated non-negative integers')
    return values


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--stages', default='tcp,http,api,files,mixed')
    parser.add_argument('--build', type=Path, default=ROOT / 'build/mingw-release')
    parser.add_argument('--source', type=Path, default=ROOT, help='Disk source matching the API build')
    parser.add_argument('--http-build', type=Path, help='Separate framework benchmark build directory')
    parser.add_argument('--framework-source', type=Path, help='gin_cpp source for provenance recording')
    parser.add_argument('--output', type=Path, help='New directory for this run (must not already exist)')
    parser.add_argument('--storage-build', type=Path, default=ROOT / 'build/storage-linux')
    parser.add_argument('--k6', type=Path, default=ROOT / 'build/tools/k6-v2.4.0-windows-amd64/k6.exe')
    parser.add_argument('--mysql-bin', default='C:/Program Files/MySQL/MySQL Server 8.0/bin')
    parser.add_argument('--distro', default='Ubuntu')
    parser.add_argument('--duration', type=int, default=60)
    parser.add_argument('--warmup', type=int, default=30)
    parser.add_argument('--repeats', type=int, default=3)
    parser.add_argument('--connections', type=numbers, default=[1, 10, 100])
    parser.add_argument('--io-threads', type=numbers, default=[1, 2, 4])
    parser.add_argument('--http-threads', type=int, default=2)
    parser.add_argument('--client-threads', type=int, default=4)
    parser.add_argument('--short-vus', type=int, default=10)
    parser.add_argument('--tcp-extras', action='store_true')
    parser.add_argument('--message-bytes', type=numbers, default=[64, 1024, 16384, 65536])
    parser.add_argument('--api-vus', type=numbers, default=[1, 10, 50])
    parser.add_argument('--cases', default='profile,quota,root,children,contents,search,trash,livez,readyz,login,register,workflow')
    parser.add_argument('--rates', type=numbers, default=[100, 500])
    parser.add_argument('--rate-vus', type=int)
    parser.add_argument('--accounts', type=int, default=10)
    parser.add_argument('--dataset-size', type=int, default=100)
    parser.add_argument('--file-bytes', type=numbers, default=[0, 1048576, 33554432, 268435456])
    parser.add_argument('--soak-seconds', type=int, default=1800)
    parser.add_argument('--recovery', type=int, default=30)
    parser.add_argument('--skip-integration', action='store_true')
    args = parser.parse_args()
    args.stages = args.stages.split(',')
    args.cases = args.cases.split(',')
    if set(args.stages) - {'tcp', 'http', 'api', 'files', 'mixed', 'soak'}:
        parser.error('Unknown stage')
    if set(args.cases) - {'profile', 'quota', 'root', 'children', 'contents', 'search', 'trash', 'livez', 'readyz',
                         'login', 'register', 'workflow', 'upload', 'download', 'mixed', 'login_mixed'}:
        parser.error('Unknown API case')
    if min(args.duration, args.repeats, args.accounts, args.soak_seconds) < 1 or min(args.warmup, args.recovery, args.dataset_size) < 0:
        parser.error('Invalid duration, repeat count, account count or dataset size')
    if any(not value for value in args.connections + args.io_threads + args.api_vus + args.message_bytes):
        parser.error('Connections, threads, VUs and message bytes must be positive')
    if not 1 <= args.http_threads <= 64 or not 1 <= args.client_threads <= 64 or args.short_vus < 1:
        parser.error('Use 1..64 HTTP/client threads and positive short-connection VUs')
    if args.rate_vus is not None and args.rate_vus < 1:
        parser.error('Rate VUs must be positive')
    args.build = args.build.resolve()
    args.source = args.source.resolve()
    args.http_build = (args.http_build or args.build).resolve()
    if args.framework_source:
        args.framework_source = args.framework_source.resolve()
    args.storage_build = args.storage_build.resolve()
    args.k6 = args.k6.resolve()
    if not args.k6.exists() and set(args.stages) != {'tcp'}:
        parser.error('Specify --k6 pointing to a k6 executable')
    directory = args.output or ROOT / 'build/performance' / time.strftime('%Y%m%d-%H%M%S')
    directory = directory.resolve()
    directory.mkdir(parents=True)
    cache = (args.build / 'CMakeCache.txt').read_text(encoding='utf-8')
    configuration = next(line.split('=', 1)[1] for line in cache.splitlines() if line.startswith('CMAKE_BUILD_TYPE:'))
    grpc_directory = next(line.split('=', 1)[1] for line in cache.splitlines() if line.startswith('gRPC_DIR:'))
    compiler = next(line.split('=', 1)[1] for line in cache.splitlines() if line.startswith('CMAKE_CXX_COMPILER:'))
    os.environ['PATH'] = str(Path(compiler).parent) + os.pathsep + os.environ['PATH']
    storage_cache = args.storage_build / 'CMakeCache.txt'
    storage_configuration = 'unknown'
    if storage_cache.exists():
        storage_configuration = next((line.split('=', 1)[1] for line in storage_cache.read_text(encoding='utf-8').splitlines()
                                      if line.startswith('CMAKE_BUILD_TYPE:')), 'unknown')
    metadata = {'commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=args.source, text=True).strip(),
                'platform': platform.platform(), 'processor': platform.processor(), 'logical_cpus': os.cpu_count(),
                'configuration': configuration, 'compiler': compiler, 'grpc_cmake_directory': grpc_directory,
                'storage_configuration': storage_configuration,
                'command': [sys.executable, *sys.argv],
                'source_directory': str(args.source),
                'args': {key: str(value) if isinstance(value, Path) else value for key, value in vars(args).items()}}
    if os.name == 'nt':
        import winreg
        with winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE, r'HARDWARE\DESCRIPTION\System\CentralProcessor\0') as cpu_key:
            metadata['cpu_model'] = winreg.QueryValueEx(cpu_key, 'ProcessorNameString')[0].strip()
        class MemoryStatus(ctypes.Structure):
            _fields_ = [('length', wintypes.DWORD), ('load', wintypes.DWORD)] + [
                (name, ctypes.c_ulonglong) for name in ('physical', 'available', 'pagefile', 'available_pagefile',
                                                       'virtual', 'available_virtual', 'extended')]
        memory = MemoryStatus()
        memory.length = ctypes.sizeof(memory)
        if ctypes.windll.kernel32.GlobalMemoryStatusEx(ctypes.byref(memory)):
            metadata['physical_memory_bytes'] = memory.physical
    save(directory / 'metadata.json', metadata)
    for name, build in [('api', args.build), ('http', args.http_build), ('storage', args.storage_build)]:
        cache_path = build / 'CMakeCache.txt'
        if cache_path.exists():
            (directory / f'{name}-CMakeCache.txt').write_bytes(cache_path.read_bytes())
    (directory / 'changes.diff').write_bytes(subprocess.check_output(['git', 'diff', 'HEAD'], cwd=args.source))
    sources = [path for base in (args.source / 'src', args.source / 'storage/src', args.source / 'storage/proto', args.source / 'tests')
               for path in base.rglob('*') if path.is_file() and path.suffix in ('.cpp', '.h', '.py', '.js', '.proto')]
    sources.extend([args.source / 'CMakeLists.txt', args.source / 'scripts/build.ps1', args.source / 'cmake/Dependencies.cmake'])
    save(directory / 'source-sha256.json', {str(path.relative_to(args.source)): hashlib.sha256(path.read_bytes()).hexdigest()
                                          for path in sources})
    provenance = {}
    network_root = next((line.split('=', 1)[1] for line in cache.splitlines()
                         if line.startswith('MOUDU_ASIO_ROOT:')), None)
    for name, source in [('disk', args.source), ('harness', ROOT), ('gin_cpp', args.framework_source),
                         ('MouduAsio', Path(network_root) if network_root else None)]:
        if source is None:
            continue
        provenance[name] = {
            'commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip(),
            'status': subprocess.check_output(['git', 'status', '--short'], cwd=source, text=True),
        }
    save(directory / 'provenance.json', provenance)
    harness_sources = [path for path in HERE.rglob('*') if path.suffix in ('.py', '.js', '.cpp', '.txt', '.ps1')]
    save(directory / 'harness-sha256.json', {str(path.relative_to(HERE)): hashlib.sha256(path.read_bytes()).hexdigest()
                                           for path in harness_sources if path.is_file()})
    if args.framework_source:
        framework_sources = [path for base in ('gin', 'third_party/moudu')
                             for path in (args.framework_source / base).rglob('*')
                             if path.is_file() and path.suffix in ('.cpp', '.h', '.txt')]
        save(directory / 'framework-sha256.json', {
            str(path.relative_to(args.framework_source)): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in framework_sources})
    if network_root:
        network_sources = [path for path in (Path(network_root) / 'net').rglob('*')
                           if path.is_file() and path.suffix in ('.cpp', '.h', '.txt')]
        save(directory / 'network-sha256.json', {
            str(path.relative_to(network_root)): hashlib.sha256(path.read_bytes()).hexdigest()
            for path in network_sources})
    binaries = [args.build / 'disk_api.exe', args.http_build / 'disk_http_benchmark.exe', args.k6,
                args.storage_build / 'grpc_storage_node']
    save(directory / 'binary-sha256.json', {str(path): hashlib.sha256(path.read_bytes()).hexdigest()
                                          for path in binaries if path.is_file()})
    resources = Resources(directory)
    results = []
    failure = None
    print(f'Output {directory}', flush=True)
    try:
        if 'tcp' in args.stages:
            run_tcp(args, directory, resources, results)
        if 'http' in args.stages:
            run_http(args, directory, resources, results)
        if set(args.stages) & {'api', 'files', 'mixed', 'soak'}:
            run_api(args, directory, resources, results)
    except Exception as error:
        if getattr(error, 'result', None):
            results.append(error.result)
        failure = str(error)
        raise
    finally:
        resources.close()
        report(directory, metadata, results, failure)
        print(f'Report {directory / "report.md"}', flush=True)
    if any(result.get('exit_code', 0) != 0 or result.get('errors', 0) or result.get('cleanup_errors', 0) for result in results):
        raise SystemExit('One or more load scenarios failed; inspect report.md')


if __name__ == '__main__':
    main()
