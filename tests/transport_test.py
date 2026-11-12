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

        for user in range(1, 9):
            with socket.create_connection(('127.0.0.1', port), timeout=5) as stream:
                authorization = token_for_user(user)
                stream.sendall(f'POST /api/files HTTP/1.1\r\nHost: test\r\nAuthorization: Bearer {authorization}\r\nContent-Length: 1000000\r\n\r\n'.encode() + b'aborted')
        # Use distinct users so per-user admission limits do not mask worker cancellation.
        upload(False, token_for_user(100))
        print('PASS keep-alive, fixed/chunked/concurrent uploads, streamed download, malformed HTTP, auth, cancellation')
    finally:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


if __name__ == '__main__':
    run(sys.argv[1])
