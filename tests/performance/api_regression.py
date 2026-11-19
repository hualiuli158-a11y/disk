"""Run the source branch's API regression with HTTP case-insensitive header lookup."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys


def main():
    source, binary, output = (Path(value).resolve() for value in sys.argv[1:])
    destination = output / 'integration-source'
    destination.mkdir()
    changes = {}
    for name in ('api_integration.py', 'file_management_cases.py'):
        original_bytes = (source / 'tests' / name).read_bytes()
        original = original_bytes.decode('utf-8').replace('\r\n', '\n')
        adapted = original
        if name == 'api_integration.py':
            old = 'result = (data, dict(response.getheaders()))'
            new = ('from email.message import Message\n'
                   '        header_fields = Message()\n'
                   '        for key, value in response.getheaders():\n'
                   '            header_fields[key] = value\n'
                   '        result = (data, header_fields)')
            if original.count(old) != 1:
                raise RuntimeError('API regression changed; review the header adaptation before running')
            adapted = original.replace(old, new)
            captured = 'stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True'
            drained = 'stdout=sys.stdout, stderr=sys.stderr, text=True'
            if adapted.count(captured) != 1 or adapted.count('errors = process.stderr.read()') != 1:
                raise RuntimeError('API regression changed; review the log redirection before running')
            adapted = adapted.replace(captured, drained).replace(
                'errors = process.stderr.read()', "errors = ''  # logs are streamed to the regression log")
        executed = destination / name
        executed.write_text(adapted, encoding='utf-8', newline='\n')
        changes[name] = {'original_sha256': hashlib.sha256(original_bytes).hexdigest(),
                         'executed_sha256': hashlib.sha256(executed.read_bytes()).hexdigest(),
                         'change': 'Case-insensitive header lookup; stream child logs instead of undrained PIPE; all assertions retained'
                                   if adapted != original else 'unchanged'}
    (output / 'integration-adaptation.json').write_text(json.dumps(changes, indent=2), encoding='utf-8')
    result = subprocess.run([sys.executable, '-u', str(destination / 'api_integration.py'), str(binary)],
                            stdout=sys.stdout, stderr=sys.stderr,
                            creationflags=subprocess.CREATE_NO_WINDOW if sys.platform == 'win32' else 0)
    raise SystemExit(result.returncode)


if __name__ == '__main__':
    main()
