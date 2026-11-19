"""Business regressions for rename, move, search, trash and shared object deletion."""
import concurrent.futures
import json
import os
from pathlib import Path
import uuid
from urllib.parse import urlencode


def run(request, json_request, upload, sql, token, other, user, root):
    def folder(parent, name):
        return json_request('POST', '/api/folders', {'parent_id': parent, 'name': name}, token=token)['id']

    def quota():
        return json_request('GET', '/api/quota', token=token)['used']

    def trash_entry(resource, authorization=token):
        entries = json_request('GET', '/api/trash?page_size=100', token=authorization)['items']
        return next(entry for entry in entries if entry['resource_id'] == resource)

    def delete_file(file, authorization=token):
        request('DELETE', f'/api/files/{file}', token=authorization, expected=204)
        return trash_entry(file, authorization)['id']

    def purge(trash, authorization=token):
        request('DELETE', f'/api/trash/{trash}', token=authorization, expected=204)

    a = folder(root, 'Managed A')
    b = folder(root, 'Managed B')
    nested = folder(a, 'Nested')
    renamed = json_request('PATCH', f'/api/folders/{a}', {'name': ' Managed Alpha '}, token=token)
    assert renamed == {'id': a, 'name': 'Managed Alpha'}
    json_request('PATCH', f'/api/folders/{a}', {'name': 'Managed Alpha'}, token=token)
    request('PATCH', f'/api/folders/{b}', {'name': 'Managed Alpha'}, token=token, expected=409)
    request('PATCH', f'/api/folders/{a}', {'name': 'private'}, token=other, expected=404)
    request('DELETE', f'/api/folders/{a}', token=other, expected=404)
    request('PATCH', f'/api/folders/{root}', {'name': 'renamed-root'}, token=token, expected=400)
    request('PATCH', f'/api/folders/{root}/move', {'parent_id': a}, token=token, expected=400)
    request('DELETE', f'/api/folders/{root}', token=token, expected=400)
    request('PATCH', f'/api/folders/{a}/move', {'parent_id': a}, token=token, expected=400)
    request('PATCH', f'/api/folders/{a}/move', {'parent_id': nested}, token=token, expected=400)
    json_request('PATCH', f'/api/folders/{a}/move', {'parent_id': b}, token=token)
    json_request('PATCH', f'/api/folders/{a}/move', {'parent_id': b}, token=token)
    request('PATCH', f'/api/folders/{b}/move', {'parent_id': nested}, token=token, expected=400)
    other_root = json_request('GET', '/api/folders/root/children', token=other)['root']['id']
    request('PATCH', f'/api/folders/{a}/move', {'parent_id': other_root}, token=token, expected=404)

    payload = uuid.uuid4().bytes + b'file-management-content'
    item = upload(token, a, 'original.txt', payload)['id']
    renamed = json_request('PATCH', f'/api/files/{item}', {'name': ' renamed.txt '}, token=token)
    assert renamed == {'id': item, 'name': 'renamed.txt'}
    json_request('PATCH', f'/api/files/{item}', {'name': 'renamed.txt'}, token=token)
    occupied = upload(token, b, 'renamed.txt', b'occupied')['id']
    request('PATCH', f'/api/files/{item}/move', {'folder_id': b}, token=token, expected=409)
    json_request('PATCH', f'/api/files/{occupied}', {'name': 'occupied.txt'}, token=token)
    assert json_request('PATCH', f'/api/files/{item}/move', {'folder_id': b}, token=token) == {'id': item, 'folder_id': b}
    json_request('PATCH', f'/api/files/{item}/move', {'folder_id': b}, token=token)
    request('PATCH', f'/api/files/{item}/move', {'folder_id': other_root}, token=token, expected=404)
    request('PATCH', f'/api/files/{item}', {'name': 'private'}, token=other, expected=404)
    request('DELETE', f'/api/files/{item}', token=other, expected=404)

    result = json_request('GET', '/api/search?' + urlencode({'q': ' Managed ', 'type': 'folder', 'page_size': 1, 'sort': 'name'}), token=token)
    assert result['total'] == 2 and result['total_pages'] == 2 and len(result['items']) == 1
    assert result['query'] == 'Managed' and result['type'] == 'folder'
    assert json_request('GET', '/api/search?q=renamed&type=file', token=token)['items'][0]['id'] == item
    assert json_request('GET', '/api/search?q=Managed', token=other)['total'] == 0
    for query in ({'q': ''}, {'q': 'x', 'type': 'bad'}, {'q': 'x', 'sort': 'name;DROP TABLE files'}, {'q': 'x', 'page_size': 101}):
        request('GET', '/api/search?' + urlencode(query), token=token, expected=400)
    assert json_request('GET', '/api/search?' + urlencode({'q': "' OR 1=1 --"}), token=token)['total'] == 0

    before = quota()
    entry = delete_file(item)
    assert quota() == before
    request('GET', f'/api/files/{item}/download', token=token, expected=404)
    assert json_request('GET', '/api/search?q=renamed', token=token)['total'] == 0
    record = trash_entry(item)
    assert record['type'] == 'file' and record['original_parent_id'] == b
    assert record['expires_at'] > record['trashed_at']
    assert json_request('GET', '/api/trash', token=other)['total'] == 0
    request('POST', f'/api/trash/{entry}/restore', token=other, expected=404)
    request('DELETE', f'/api/trash/{entry}', token=other, expected=404)
    replacement = upload(token, b, 'renamed.txt', uuid.uuid4().bytes)['id']
    request('POST', f'/api/trash/{entry}/restore', token=token, expected=409)
    request('GET', f'/api/files/{item}/download', token=token, expected=404)
    json_request('PATCH', f'/api/files/{replacement}', {'name': 'replacement.txt'}, token=token)
    request('POST', f'/api/trash/{entry}/restore', token=token, expected=204)
    assert request('GET', f'/api/files/{item}/download', token=token)[0] == payload
    shared = upload(other, other_root, 'shared.txt', payload)['id']
    before = quota()
    entry = delete_file(item)
    purge(entry)
    assert quota() == before - len(payload)
    request('DELETE', f'/api/trash/{entry}', token=token, expected=404)
    assert request('GET', f'/api/files/{shared}/download', token=other)[0] == payload
    purge(delete_file(shared, other), other)

    restored = upload(token, nested, 'fallback.txt', uuid.uuid4().bytes)['id']
    child_entry = delete_file(restored)
    request('DELETE', f'/api/folders/{a}', token=token, expected=204)
    parent_entry = trash_entry(a)['id']
    request('GET', f'/api/folders/{nested}/contents', token=token, expected=404)
    request('POST', f'/api/trash/{child_entry}/restore', token=token, expected=204)
    root_files = json_request('GET', f'/api/folders/{root}/contents', token=token)['items']
    assert any(file['id'] == restored and file['type'] == 'file' for file in root_files)
    request('POST', f'/api/trash/{parent_entry}/restore', token=token, expected=204)
    json_request('GET', f'/api/folders/{nested}/contents', token=token)
    hidden_data = uuid.uuid4().bytes + b'hidden'
    hidden = upload(token, nested, 'hidden.txt', hidden_data)['id']
    hidden_entry = delete_file(hidden)
    request('DELETE', f'/api/folders/{a}', token=token, expected=204)
    parent_entry = trash_entry(a)['id']
    before = quota()
    purge(parent_entry)
    assert quota() == before - len(hidden_data)
    request('POST', f'/api/trash/{hidden_entry}/restore', token=token, expected=404)
    request('GET', f'/api/folders/{nested}/contents', token=token, expected=404)
    request('GET', f'/api/files/{restored}/download', token=token)

    conflict = folder(root, 'Restore conflict')
    conflict_child = folder(conflict, 'Child')
    conflict_file = upload(token, conflict_child, 'conflict.txt', b'conflict')['id']
    request('DELETE', f'/api/folders/{conflict}', token=token, expected=204)
    conflict_entry = trash_entry(conflict)['id']
    replacement_folder = folder(root, 'Restore conflict')
    request('POST', f'/api/trash/{conflict_entry}/restore', token=token, expected=409)
    request('GET', f'/api/files/{conflict_file}/download', token=token, expected=404)
    json_request('PATCH', f'/api/folders/{replacement_folder}', {'name': 'Restore replacement'}, token=token)
    request('POST', f'/api/trash/{conflict_entry}/restore', token=token, expected=204)
    assert request('GET', f'/api/files/{conflict_file}/download', token=token)[0] == b'conflict'
    request('GET', '/api/trash?page=0', token=token, expected=400)
    request('GET', '/api/trash?page_size=101', token=token, expected=400)

    cycle_a = folder(root, 'Cycle A')
    cycle_b = folder(root, 'Cycle B')
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        jobs = [pool.submit(request, 'PATCH', f'/api/folders/{source}/move', {'parent_id': target}, token, None, (200, 400))
                for source, target in ((cycle_a, cycle_b), (cycle_b, cycle_a))]
        results = [json.loads(job.result()[0]) for job in jobs]
        assert sum('id' in result for result in results) == 1

    if os.environ.get('MYSQL_CLIENT'):
        history_a = upload(token, root, 'history-a.bin', uuid.uuid4().bytes)['id']
        history_b = upload(token, root, 'history-b.bin', uuid.uuid4().bytes)['id']
        physical_a = int(sql(f'SELECT physical_file_id FROM files WHERE id={history_a}'))
        physical_b = int(sql(f'SELECT physical_file_id FROM files WHERE id={history_b}'))
        key_a = sql(f'SELECT storage_key FROM physical_files WHERE id={physical_a}')
        sql(f"INSERT INTO file_versions(file_id,physical_file_id,source,created_at) VALUES({history_b},{physical_a},'edit',UTC_TIMESTAMP(3))")
        purge(delete_file(history_a))
        assert sql(f'SELECT COUNT(*) FROM physical_files WHERE id={physical_a}') == '1'
        request('GET', f'/api/files/{history_b}/download', token=token)
        purge(delete_file(history_b))
        assert sql(f'SELECT COUNT(*) FROM physical_files WHERE id IN ({physical_a},{physical_b})') == '0'
        if os.environ.get('STORAGE_TEST_ROOT'):
            assert not (Path(os.environ['STORAGE_TEST_ROOT']) / key_a).exists()
    print('PASS rename/move/search, root and ownership checks, cyclic moves, trash conflicts, recursive restore/delete, shared/history object references', flush=True)
