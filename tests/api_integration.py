"""Run against an isolated migrated MySQL database and a running storage node."""
import base64
import concurrent.futures
import hashlib
import hmac
import http.client
import json
import os
import subprocess
import sys
import tempfile
import time
import uuid


def run(executable):
    # The server logs each request, so a pipe left unread can block the server mid-test.
    server_log = tempfile.TemporaryFile(mode='w+', encoding='utf-8', errors='replace')
    process = subprocess.Popen([executable], stdout=subprocess.DEVNULL, stderr=server_log)
    port = int(os.environ.get('PORT', '8080'))
    host = '127.0.0.1'

    def request(method, path, body=None, token=None, headers=None, expected=200):
        client = http.client.HTTPConnection(host, port, timeout=30)
        outgoing = dict(headers or {})
        if token:
            outgoing['Authorization'] = f'Bearer {token}'
        if isinstance(body, dict):
            outgoing['Content-Type'] = 'application/json'
            body = json.dumps(body).encode()
        client.request(method, path, body=body, headers=outgoing)
        response = client.getresponse()
        data = response.read()
        assert response.status in (expected if isinstance(expected, tuple) else (expected,)), (method, path, response.status, data[:500])
        result = (data, response.headers)
        client.close()
        return result

    def json_request(*args, **kwargs):
        return json.loads(request(*args, **kwargs)[0])

    def sql(statement):
        environment = os.environ.copy()
        environment['MYSQL_PWD'] = environment['DB_PASSWORD']
        result = subprocess.run([
            environment['MYSQL_CLIENT'], '--no-defaults', '--protocol=TCP',
            '--host=' + environment['DB_HOST'], '--port=' + environment['DB_PORT'],
            '--user=' + environment['DB_USER'], '--database=' + environment['DB_NAME'],
            '--batch', '--raw', '--skip-column-names',
        ], input=statement, text=True, capture_output=True, env=environment, check=True)
        return result.stdout.strip()

    def upload(token, folder, name, content, expected=200, chunked=False, declared_size=None):
        boundary = 'disk-test-' + uuid.uuid4().hex
        prefix = (f'--{boundary}\r\nContent-Disposition: form-data; name="folder_id"\r\n\r\n{folder}\r\n'
                  f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{name}"\r\n'
                  'Content-Type: application/octet-stream\r\n\r\n').encode()
        suffix = f'\r\n--{boundary}--\r\n'.encode()
        client = http.client.HTTPConnection(host, port, timeout=60)
        client.putrequest('POST', '/api/files')
        client.putheader('Authorization', f'Bearer {token}')
        client.putheader('Content-Type', f'multipart/form-data; boundary="{boundary}"')
        if declared_size is not None:
            client.putheader('X-Upload-Size', str(declared_size))
        client.putheader('Transfer-Encoding' if chunked else 'Content-Length', 'chunked' if chunked else str(len(prefix) + len(content) + len(suffix)))
        client.endheaders()
        # A prime fragment length exercises multipart boundaries split across network reads.
        for source in (prefix, content, suffix):
            for index in range(0, len(source), 7919):
                block = source[index:index + 7919]
                if chunked:
                    client.send(f'{len(block):x}\r\n'.encode())
                client.send(block)
                if chunked:
                    client.send(b'\r\n')
        if chunked:
            client.send(b'0\r\n\r\n')
        response = client.getresponse()
        data = response.read()
        assert response.status in (expected if isinstance(expected, tuple) else (expected,)), (response.status, data[:500])
        client.close()
        value = json.loads(data)
        value['_status'] = response.status
        return value

    try:
        for _ in range(100):
            if process.poll() is not None:
                server_log.seek(0)
                raise RuntimeError(server_log.read())
            try:
                request('GET', '/livez')
                break
            except OSError:
                time.sleep(0.1)
        else:
            raise RuntimeError('API failed to start')

        if os.environ.get('SWAGGER_ENABLED') == 'true':
            html, headers = request('GET', '/docs/')
            assert headers.get_content_type() == 'text/html' and b'id="swagger-ui"' in html
            bundle, headers = request('GET', '/docs/swagger-ui-bundle.js')
            assert headers.get_content_type() == 'application/javascript' and b'SwaggerUIBundle' in bundle
            spec = json_request('GET', '/openapi.json')
            assert spec['security'] == [{'BearerAuth': []}]
            assert spec['paths']['/login']['post']['security'] == []
            upload_content = spec['paths']['/api/files']['post']['requestBody']['content']
            assert upload_content['multipart/form-data']['schema']['required'] == ['folder_id', 'file']
        else:
            request('GET', '/docs/', expected=404)
            request('GET', '/openapi.json', expected=404)

        username = 'cpp_' + uuid.uuid4().hex[:16]
        credentials = {'username': username, 'password': 'correct-password-123'}
        token = json_request('POST', '/register', credentials)['access_token']
        json_request('POST', '/register', credentials, expected=409)
        login = json_request('POST', '/login', credentials)['access_token']
        profile = json_request('GET', '/api/profile', token=login)
        user = profile['id']
        root = json_request('GET', '/api/folders/root/children', token=token)['root']['id']
        folder = json_request('POST', '/api/folders', {'parent_id': root, 'name': 'uploads'}, token=token)['id']
        json_request('POST', '/api/folders', {'parent_id': root, 'name': 'uploads'}, token=token, expected=409)
        assert json_request('GET', '/api/quota', token=token)['used'] == 0

        def early_upload(folder_id, declared_size, expected):
            boundary = 'preflight-' + uuid.uuid4().hex
            client = http.client.HTTPConnection(host, port, timeout=10)
            client.putrequest('POST', '/api/files')
            client.putheader('Authorization', f'Bearer {token}')
            client.putheader('Content-Type', f'multipart/form-data; boundary={boundary}')
            client.putheader('Content-Length', '1000000')
            client.putheader('X-Upload-Size', str(declared_size))
            client.endheaders()
            client.send((f'--{boundary}\r\nContent-Disposition: form-data; name="folder_id"\r\n\r\n{folder_id}\r\n'
                         f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="preflight.bin"\r\n\r\npartial').encode())
            response = client.getresponse()
            assert response.status == expected, (response.status, response.read())
            response.read()
            client.close()

        early_upload(18446744073709551615, 100, 404)
        early_upload(folder, -1, 400)
        early_upload(folder, 1073741825, 413)
        root_listing = json_request('GET', '/api/folders/root/children', token=token)
        assert root_listing['root']['parent_id'] is None
        assert [child['id'] for child in root_listing['children']] == [folder]
        children = json_request('GET', f'/api/folders/{root}/children', token=token)
        assert children['folder'] == root_listing['root'] and children['children'] == root_listing['children']
        empty_listing = json_request('GET', f'/api/folders/{folder}/contents', token=token)
        assert empty_listing == {'items': [], 'page': 1, 'page_size': 100, 'total': 0, 'total_pages': 0}
        for query in ('page=0', 'page_size=101', 'sort=invalid', 'order=invalid'):
            request('GET', f'/api/folders/{folder}/contents?{query}', token=token, expected=400)
        root_contents = json_request('GET', f'/api/folders/{root}/contents', token=token)['items']
        assert len(root_contents) == 1 and root_contents[0]['type'] == 'folder'
        assert 'size' not in root_contents[0] and 'sha256' not in root_contents[0]

        content = (b'File bytes\x00\xff\r\n' * 100003) + b'last'
        print('PASS account, JWT login, folders and quota. Testing upload...', flush=True)
        first = upload(token, folder, 'data.bin', content)
        assert first['size'] == len(content)
        assert first['sha256'] == hashlib.sha256(content).hexdigest()
        download, headers = request('GET', f'/api/files/{first["id"]}/download', token=token)
        assert download == content
        assert headers['ETag'] == f'"{first["sha256"]}"'
        assert headers.get_all('Content-Type') == ['application/octet-stream']
        upload(token, folder, 'data.bin', content, expected=409)
        second = upload(token, folder, 'same-content.bin', content, chunked=True)
        assert second['id'] != first['id']
        assert json_request('GET', '/api/quota', token=token)['used'] == len(content) * 2
        empty = upload(token, folder, 'empty.txt', b'', declared_size=0)
        assert request('GET', f'/api/files/{empty["id"]}/download', token=token)[0] == b''
        listing = json_request('GET', f'/api/folders/{folder}/contents?page_size=2&page=1&sort=name&order=asc', token=token)
        assert listing['total'] == 3 and len(listing['items']) == 2
        assert isinstance(listing['items'][0]['created_at'], str)
        full_listing = json_request('GET', f'/api/folders/{folder}/contents', token=token)['items']
        empty_item = next(item for item in full_listing if item['id'] == empty['id'])
        assert empty_item['size'] == 0 and empty_item['sha256'] == hashlib.sha256(b'').hexdigest()

        other = json_request('POST', '/register', {'username': username + '_other', 'password': 'another-password'})['access_token']
        request('GET', f'/api/files/{first["id"]}/download', token=other, expected=404)
        request('GET', f'/api/folders/{folder}/contents', token=other, expected=404)
        request('GET', f'/api/folders/{folder}/contents?page=0&sort=invalid', token=other, expected=404)
        request('GET', f'/api/folders/{folder}/children', token=other, expected=404)
        request('GET', '/api/profile', expected=401)
        request('GET', '/api/profile', token=token + 'tamper', expected=401)

        def signed(claims):
            def encode(value):
                return base64.urlsafe_b64encode(json.dumps(value, separators=(',', ':')).encode()).rstrip(b'=')
            parts = encode({'alg': 'HS256', 'typ': 'JWT'}) + b'.' + encode(claims)
            signature = hmac.new(os.environ['JWT_SECRET'].encode(), parts, hashlib.sha256).digest()
            return (parts + b'.' + base64.urlsafe_b64encode(signature).rstrip(b'=')).decode()

        claims = {'user_id': user, 'iss': os.environ.get('JWT_ISSUER', 'disk'), 'exp': int(time.time()) + 100}
        request('GET', '/api/profile', token=signed(claims))
        claims['exp'] = 1
        request('GET', '/api/profile', token=signed(claims), expected=401)

        unicode_name = '文' * 255
        unicode_folder = json_request('POST', '/api/folders', {'parent_id': root, 'name': '\u3000' + unicode_name + '\u3000'}, token=token)
        assert unicode_folder['name'] == unicode_name
        unicode_file = upload(token, unicode_folder['id'], unicode_name, b'')
        assert unicode_file['name'] == unicode_name

        from file_management_cases import run as management_tests
        management_tests(request, json_request, upload, sql, token, other, user, root)

        if os.environ.get('MYSQL_CLIENT'):
            import bcrypt
            stored_hash = sql(f'SELECT password FROM users WHERE id={user}')
            assert bcrypt.checkpw(credentials['password'].encode(), stored_hash.encode())
            foreign_hash = bcrypt.hashpw(b'foreign-password-123', bcrypt.gensalt(rounds=10, prefix=b'2a')).decode()
            fixture_name = username + '_bcrypt'
            sql(f"INSERT INTO users(username,password,create_time) VALUES('{fixture_name}','{foreign_hash}',UTC_TIMESTAMP(3))")
            json_request('POST', '/login', {'username': fixture_name, 'password': 'foreign-password-123'})
            assert sql(f"SELECT COUNT(*) FROM physical_files WHERE sha256='{first['sha256']}'") == '1'
            used = json_request('GET', '/api/quota', token=token)['used']
            data = b'q' * (1024 * 1024)
            sql(f'UPDATE users SET quota_limit={used + len(data)} WHERE id={user}')
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                jobs = [pool.submit(upload, token, folder, f'quota-{i}.bin', data, (200, 507)) for i in range(2)]
                assert sorted(job.result()['_status'] for job in jobs) == [200, 507]
            assert json_request('GET', '/api/quota', token=token)['used'] == used + len(data)
            early_upload(folder, 1, 507)
            print('PASS bcrypt interoperability, database dedup and concurrent quota enforcement', flush=True)
        print('PASS real MySQL + gRPC storage: registration/login, folders, binary/chunked/empty uploads, download/hash, dedup, quota, pagination, ownership, JWT')
    finally:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        server_log.seek(0)
        errors = server_log.read()
        server_log.close()
        if errors:
            print(errors, file=sys.stderr)


if __name__ == '__main__':
    run(sys.argv[1])
