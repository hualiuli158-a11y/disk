import concurrent.futures
import base64
import hashlib
import hmac
import http.client
import json
import socket
import subprocess
import sys
import time
import threading


def run(executable):
    process = subprocess.Popen([executable], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        line = process.stdout.readline().strip()
        assert line.startswith('READY '), line
        _, port, token = line.split()
        port = int(port)

        def connection():
            return http.client.HTTPConnection('127.0.0.1', port, timeout=30)

        client = connection()
        for _ in range(3):
            client.request('POST', '/echo', body=b'hello')
            response = client.getresponse()
            assert response.status == 200
            assert json.load(response)['body'] == 'hello'
        client.close()

        def token_for_user(user):
            def encode(value):
                return base64.urlsafe_b64encode(json.dumps(value, separators=(',', ':')).encode()).rstrip(b'=')
            payload = encode({'alg': 'HS256', 'typ': 'JWT'}) + b'.' + encode({
                'user_id': user, 'iss': 'disk', 'exp': int(time.time()) + 900,
            })
            signature = hmac.new(b't' * 48, payload, hashlib.sha256).digest()
            return (payload + b'.' + base64.urlsafe_b64encode(signature).rstrip(b'=')).decode()

        def upload(chunked, authorization=token):
            client = connection()
            length = 8 * 1024 * 1024 + 17
            block = b'u' * 65536
            client.putrequest('POST', '/api/files')
            client.putheader('Authorization', f'Bearer {authorization}')
            client.putheader('Transfer-Encoding' if chunked else 'Content-Length', 'chunked' if chunked else str(length))
            client.endheaders()
            sent = 0
            while sent < length:
                data = block[:min(len(block), length - sent)]
                if chunked:
                    client.send(f'{len(data):x}\r\n'.encode())
                client.send(data)
                if chunked:
                    client.send(b'\r\n')
                sent += len(data)
            if chunked:
                client.send(b'0\r\n\r\n')
            response = client.getresponse()
            assert response.status == 200, response.read()
            assert json.load(response)['size'] == length
            client.close()

        upload(False)
        upload(True)
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            list(pool.map(upload, [False, True]))

        client = connection()
        client.request('GET', '/download')
        response = client.getresponse()
        digest = hashlib.sha256()
        length = 0
        while data := response.read(8192):
            digest.update(data)
            length += len(data)
        assert length == 32 * 1024 * 1024
        assert digest.digest() == hashlib.sha256(b'd' * length).digest()
        client.close()

        def active_uploads():
            client = connection()
            client.request('GET', '/upload-status')
            response = client.getresponse()
            assert response.status == 200
            value = json.load(response)['active']
            client.close()
            return value

        def wait_active(expected):
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                if active_uploads() == expected:
                    return
                time.sleep(0.02)
            raise AssertionError(f'Active upload count did not reach {expected}')

        def wait_capacity():
            deadline = time.monotonic() + 5
            while time.monotonic() < deadline:
                client = connection()
                client.request('POST', '/api/files', body=b'', headers={'Authorization': f'Bearer {token_for_user(999)}'})
                response = client.getresponse()
                status = response.status
                response.read()
                client.close()
                if status == 200:
                    return
                assert status == 429
                time.sleep(0.02)
            raise AssertionError('Upload admission slots were not released')

        slow_uploads = []
        heartbeat_stop = threading.Event()
        heartbeat = None
        try:
            for user in range(201, 205):
                stream = socket.create_connection(('127.0.0.1', port), timeout=5)
                stream.sendall(f'POST /api/files HTTP/1.1\r\nHost: test\r\nAuthorization: Bearer {token_for_user(user)}\r\nContent-Length: 1000000\r\n\r\nslow'.encode())
                slow_uploads.append(stream)

            def keep_uploads_alive():
                while not heartbeat_stop.wait(0.1):
                    for stream in slow_uploads:
                        try:
                            stream.sendall(b'.')
                        except OSError:
                            return

            heartbeat = threading.Thread(target=keep_uploads_alive, daemon=True)
            heartbeat.start()
            wait_active(4)
            client = http.client.HTTPConnection('127.0.0.1', port, timeout=2)
            client.request('GET', '/download')
            response = client.getresponse()
            assert response.status == 200
            assert response.read(65536) == b'd' * 65536
            assert active_uploads() == 4
            client.close()
            client = connection()
            client.request('POST', '/api/files', body=b'', headers={'Authorization': f'Bearer {token_for_user(999)}'})
            response = client.getresponse()
            assert response.status == 429, response.read()
            response.read()
            client.close()
        finally:
            heartbeat_stop.set()
            if heartbeat:
                heartbeat.join(timeout=5)
            for stream in slow_uploads:
                stream.close()
        wait_active(0)
        wait_capacity()

        # A full server-side queue pauses the body idle check until consumption resumes.
        client = connection()
        client.putrequest('POST', '/api/files')
        client.putheader('Authorization', f'Bearer {token_for_user(600)}')
        client.putheader('Content-Length', str(512 * 1024))
        client.endheaders()
        client.send(b'b' * (256 * 1024))
        time.sleep(3)
        client.send(b'b' * (256 * 1024))
        response = client.getresponse()
        assert response.status == 200, response.read()
        assert json.load(response)['size'] == 512 * 1024
        client.close()

        def busy_request():
            client = connection()
            client.request('GET', '/busy')
            response = client.getresponse()
            assert response.status == 200
            response.read()
            client.close()

        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
            jobs = [pool.submit(busy_request) for _ in range(4)]
            deadline = time.monotonic() + 1.5
            while True:
                client = http.client.HTTPConnection('127.0.0.1', port, timeout=1)
                client.request('GET', '/livez')
                response = client.getresponse()
                assert response.status == 200
                count = json.load(response)['active_business']
                client.close()
                if count == 4:
                    break
                assert time.monotonic() < deadline, 'Business workers were not saturated'
                time.sleep(0.02)
            for job in jobs:
                job.result()

        # An idle request must time out and return its global admission slot.
        with socket.create_connection(('127.0.0.1', port), timeout=6) as stream:
            stream.sendall(f'POST /api/files HTTP/1.1\r\nHost: test\r\nAuthorization: Bearer {token}\r\nContent-Length: 1000000\r\n\r\nidle'.encode())
            response = http.client.HTTPResponse(stream)
            response.begin()
            assert response.status == 408
            response.read()
        wait_active(0)
        wait_capacity()
        upload(False)

        for request, status in [
            (b'GET /bad%ZZ HTTP/1.1\r\nHost: test\r\n\r\n', 400),
            (b'POST /api/files HTTP/1.1\r\nHost: test\r\nContent-Length: 1000000\r\n\r\n', 401),
            (b'POST /echo HTTP/1.1\r\nHost: test\r\nContent-Length: 1000000\r\n\r\n', 413),
            (b'POST /echo HTTP/1.1\r\nHost: test\r\nContent-Length: 1\r\nTransfer-Encoding: chunked\r\n\r\n', 400),
        ]:
            with socket.create_connection(('127.0.0.1', port), timeout=5) as stream:
                stream.sendall(request)
                reply = http.client.HTTPResponse(stream)
                reply.begin()
                assert reply.status == status, (reply.status, status)
                reply.read()

        print('PASS streaming, slow-upload/download isolation, health isolation, global admission, idle timeout, backpressure, cancellation')
    finally:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


if __name__ == '__main__':
    run(sys.argv[1])
