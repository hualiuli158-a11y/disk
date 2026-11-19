"""Start a fresh Docker-backed environment and run the Disk C++ API suite."""

import argparse
from contextlib import contextmanager
import os
from pathlib import Path
import secrets
import socket
import subprocess
import uuid

import pymysql
from pymysql.constants import CLIENT
from testcontainers.community.mysql import MySqlContainer
from testcontainers.core.container import DockerContainer
from testcontainers.core.network import Network
from testcontainers.core.wait_strategies import LogMessageWaitStrategy

from api_integration import run

PROJECT_ROOT = Path(__file__).resolve().parent.parent
LOOPBACK = '127.0.0.1'
MYSQL_IMAGE = 'mysql:8.0.42'
STORAGE_PORT = 50051


def apply_migrations(connection):
    migrations = sorted((PROJECT_ROOT / 'migrations').glob('*.up.sql'))
    if not migrations:
        raise RuntimeError('No database migrations found')
    with connection.cursor() as cursor:
        for migration in migrations:
            cursor.execute(migration.read_text(encoding='utf-8'))
            while cursor.nextset():
                pass


def api_port():
    with socket.socket() as listener:
        listener.bind((LOOPBACK, 0))
        return listener.getsockname()[1]


@contextmanager
def build_storage_image(run_id):
    tag = f'disk-integration-storage:{run_id}'
    subprocess.run([
        'docker', 'build', '--progress=plain', '--tag', tag,
        '--label', f'disk.integration.run={run_id}', str(PROJECT_ROOT / 'storage'),
    ], check=True)
    try:
        yield tag
    finally:
        subprocess.run(['docker', 'image', 'rm', '--no-prune', tag], check=True)


def run_in_containers(executable):
    run_id = uuid.uuid4().hex
    print('Building the gRPC storage test image...', flush=True)
    with build_storage_image(run_id) as storage_image, Network(docker_network_kw={
        'driver': 'bridge',
        'options': {'com.docker.network.bridge.host_binding_ipv4': LOOPBACK},
    }) as network:
        mysql = MySqlContainer(
            MYSQL_IMAGE, username='disk_test', dbname='disk_test',
            password=secrets.token_hex(24), root_password=secrets.token_hex(24),
        ).with_network(network)
        storage = (
            DockerContainer(storage_image)
            .with_network(network)
            .with_exposed_ports(STORAGE_PORT)
            .waiting_for(LogMessageWaitStrategy('gRPC storage listening on'))
        )
        print('Starting isolated MySQL and gRPC storage...', flush=True)
        with mysql, storage, pymysql.connect(
            host=LOOPBACK, port=int(mysql.get_exposed_port(mysql.port)),
            user=mysql.username, password=mysql.password, database=mysql.dbname,
            charset='utf8mb4', autocommit=True, client_flag=CLIENT.MULTI_STATEMENTS,
            connect_timeout=5, read_timeout=15, write_timeout=15,
        ) as database:
            apply_migrations(database)

            def sql(statement):
                with database.cursor() as cursor:
                    cursor.execute(statement)
                    row = cursor.fetchone()
                    return '' if row is None else str(row[0])

            def stored_object_exists(key):
                result = storage.get_wrapped_container().exec_run(['test', '-e', '/data/' + key])
                if result.exit_code not in (0, 1):
                    raise RuntimeError(f'Could not inspect storage object {key}: {result.output!r}')
                return result.exit_code == 0

            environment = {
                key: value for key, value in os.environ.items()
                if not key.startswith(('DB_', 'JWT_', 'STORAGE_', 'UPLOAD_'))
                and key != 'MYSQL_CLIENT'
            }
            environment.update(
                LISTEN=LOOPBACK, PORT=str(api_port()),
                DB_HOST=LOOPBACK, DB_PORT=str(mysql.get_exposed_port(mysql.port)),
                DB_NAME=mysql.dbname, DB_USER=mysql.username, DB_PASSWORD=mysql.password,
                JWT_ISSUER='disk', JWT_SECRET=secrets.token_hex(32),
                STORAGE_ADDRESS=f'{LOOPBACK}:{storage.get_exposed_port(STORAGE_PORT)}',
            )
            run(executable, environment=environment, sql=sql, stored_object_exists=stored_object_exists)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('executable', type=Path, help='Path to the built C++ disk_api executable')
    args = parser.parse_args()
    if not args.executable.is_file():
        parser.error(f'Disk API executable not found: {args.executable}')
    run_in_containers(args.executable.resolve())
