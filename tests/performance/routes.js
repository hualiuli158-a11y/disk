import http from 'k6/http';
import execution from 'k6/execution';
import { sha256 } from 'k6/crypto';
import { Counter, Rate, Trend } from 'k6/metrics';

const base = (__ENV.BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const testCase = __ENV.CASE || 'profile';
const fixtures = __ENV.FIXTURES ? JSON.parse(open(__ENV.FIXTURES)) : null;
const fileData = __ENV.UPLOAD_FILE ? new Uint8Array(open(__ENV.UPLOAD_FILE, 'b')) : new Uint8Array(1024).fill(120);
const echoBody = 'x'.repeat(Number(__ENV.BODY_BYTES || 1024));
const runId = __ENV.RUN_ID || String(Date.now());
const scriptHash = sha256(open('./routes.js'), 'hex');
const virtualUsers = Number(__ENV.VUS || 10);
const uploadActors = Math.min(4, Math.max(1, Math.floor(virtualUsers / 5)),
  fixtures ? fixtures.accounts.length * 2 : 4);

const requests = new Counter('target_requests');
const successes = new Counter('target_successes');
const checks = new Rate('target_checks');
const duration = new Trend('target_duration_ms', true);
const complete = new Trend('target_complete_ms', true);
const firstByte = new Trend('target_ttfb_ms', true);
const received = new Counter('target_body_bytes');
const statuses = new Counter('target_status');

const routeNames = [
  'GET /benchmark/plaintext', 'GET /benchmark/json', 'POST /benchmark/echo',
  'POST /register', 'POST /login', 'GET /api/profile', 'GET /api/quota',
  'GET /api/folders/root/children', 'GET /api/folders/:id/children',
  'GET /api/folders/:id/contents', 'GET /api/search', 'GET /api/trash',
  'POST /api/folders', 'PATCH /api/folders/:id', 'PATCH /api/folders/:id/move',
  'DELETE /api/folders/:id', 'POST /api/files', 'GET /api/files/:id/download',
  'PATCH /api/files/:id', 'PATCH /api/files/:id/move', 'DELETE /api/files/:id',
  'POST /api/trash/:id/restore', 'DELETE /api/trash/:id', 'GET /livez', 'GET /readyz',
];
const thresholds = { target_checks: ['rate==1'] };
for (const route of routeNames) {
  // Empty thresholds expose tagged submetrics in the JSON summary.
  for (const metric of ['target_requests', 'target_successes', 'target_duration_ms', 'target_complete_ms', 'target_checks']) {
    thresholds[`${metric}{route:${route}}`] = [];
  }
}
for (const status of [0, 200, 204, 400, 401, 404, 409, 413, 429, 500, 502, 503, 504, 507]) {
  thresholds[`target_status{status:${status}}`] = [];
}
if (__ENV.P99_MS) thresholds.target_duration_ms = [`p(99)<${Number(__ENV.P99_MS)}`];

const scenario = {
  executor: 'constant-vus', vus: Number(__ENV.VUS || 10),
  duration: __ENV.DURATION || '60s', gracefulStop: '30s',
};
if (__ENV.RATE) {
  delete scenario.vus;
  scenario.executor = 'constant-arrival-rate';
  scenario.rate = Number(__ENV.RATE);
  scenario.timeUnit = '1s';
  scenario.preAllocatedVUs = Number(__ENV.VUS || 10);
  scenario.maxVUs = Number(__ENV.MAX_VUS || 200);
}
export const options = {
  scenarios: { measured: scenario }, thresholds,
  noConnectionReuse: __ENV.SHORT_CONNECTIONS === '1',
  summaryTrendStats: ['avg', 'min', 'med', 'p(95)', 'p(99)', 'max'],
  systemTags: ['name', 'method', 'status', 'scenario', 'expected_response'],
};

let account, token, authenticatedAt = 0, mixedDownload = null;

function request(method, path, body, expected, route, phase = 'target', validate = null, binary = false) {
  const headers = {};
  if (token) headers.Authorization = `Bearer ${token}`;
  if (body && body.json) {
    headers['Content-Type'] = 'application/json';
    body = JSON.stringify(body.json);
  } else if (body && body.multipart) {
    headers['Content-Type'] = `multipart/form-data; boundary=${body.boundary}`;
    headers['X-Upload-Size'] = String(body.size);
    body = body.multipart;
  }
  const response = http.request(method, base + path, body || null, {
    headers, tags: { name: route, phase }, timeout: '60s', redirects: 0,
    responseType: binary ? 'binary' : 'text',
  });
  let valid = expected.includes(response.status);
  if (valid && validate) {
    try { valid = Boolean(validate(response)); } catch (_) { valid = false; }
  }
  if (phase === 'target') {
    const tags = { route };
    requests.add(1, tags);
    successes.add(valid && response.status < 400 ? 1 : 0, tags);
    checks.add(valid, tags);
    duration.add(response.timings.duration, tags);
    complete.add(response.timings.blocked + response.timings.duration, tags);
    firstByte.add(response.timings.waiting, tags);
    statuses.add(1, { status: String(response.status) });
    if (response.body) received.add(binary ? response.body.byteLength : response.body.length, tags);
  }
  if (!valid && ['workflow', 'upload', 'mixed'].includes(testCase)) {
    execution.test.abort(`Unexpected response for ${route}, HTTP ${response.status}`);
  }
  return response;
}

function selectAccount() {
  if (!fixtures) throw new Error('FIXTURES is required for API scenarios');
  account = fixtures.accounts[(__VU - 1) % fixtures.accounts.length];
}

function authenticate() {
  selectAccount();
  if (!token) {
    token = account.token;
    authenticatedAt = account.token_at;
  }
  if (Date.now() - authenticatedAt > 600000) {
    const response = request('POST', '/login', { json: {
      username: account.username, password: fixtures.password,
    } }, [200], 'POST /login', 'authentication');
    token = response.json('access_token');
    authenticatedAt = Date.now();
  }
}

function multipart(folder, name) {
  const boundary = `disk-perf-${runId}-${__VU}-${__ITER}`;
  const prefix = `--${boundary}\r\nContent-Disposition: form-data; name="folder_id"\r\n\r\n${folder}\r\n` +
    `--${boundary}\r\nContent-Disposition: form-data; name="file"; filename="${name}"\r\n` +
    'Content-Type: application/octet-stream\r\n\r\n';
  const suffix = `\r\n--${boundary}--\r\n`;
  const data = new Uint8Array(prefix.length + fileData.length + suffix.length);
  for (let i = 0; i < prefix.length; i++) data[i] = prefix.charCodeAt(i);
  data.set(fileData, prefix.length);
  if (__ENV.CONTENT_MODE === 'unique') {
    const marker = `${runId}-${__VU}-${__ITER}`;
    for (let i = 0; i < Math.min(marker.length, fileData.length); i++) data[prefix.length + i] = marker.charCodeAt(i);
  }
  for (let i = 0; i < suffix.length; i++) data[prefix.length + fileData.length + i] = suffix.charCodeAt(i);
  return { multipart: data.buffer, boundary, size: fileData.length };
}

function trashEntry(id, phase, type = 'file') {
  const entries = request('GET', '/api/trash?page_size=100', null, [200], 'GET /api/trash', phase).json('items');
  const entry = entries.find(item => item.resource_id === id && item.type === type);
  if (!entry) execution.test.abort(`Missing trash entry for resource ${id}`);
  return entry.id;
}

function purgeFile(id, phase) {
  request('DELETE', `/api/files/${id}`, null, [204], 'DELETE /api/files/:id', phase);
  request('DELETE', `/api/trash/${trashEntry(id, phase)}`, null, [204], 'DELETE /api/trash/:id', phase);
}

function download(id, hash) {
  request('GET', `/api/files/${id}/download`, null, [200], 'GET /api/files/:id/download', 'target',
    response => sha256(response.body, 'hex') === hash, true);
}

function upload(phase = 'target') {
  const response = request('POST', '/api/files', multipart(account.folder, `perf-${runId}-${__VU}-${__ITER}.bin`),
    __ENV.ALLOW_429 === '1' ? [200, 429] : [200], 'POST /api/files', phase,
    result => result.status === 429 || result.json('size') === fileData.length);
  return response.status === 200 ? response.json() : null;
}

function workflow() {
  const unique = `${runId}-${__VU}-${__ITER}`;
  const a = request('POST', '/api/folders', { json: { parent_id: account.folder, name: `a-${unique}` } },
    [200], 'POST /api/folders').json('id');
  const b = request('POST', '/api/folders', { json: { parent_id: account.folder, name: `b-${unique}` } },
    [200], 'POST /api/folders').json('id');
  request('PATCH', `/api/folders/${a}`, { json: { name: `renamed-${unique}` } }, [200], 'PATCH /api/folders/:id');
  request('PATCH', `/api/folders/${a}/move`, { json: { parent_id: b } }, [200], 'PATCH /api/folders/:id/move');
  const file = upload();
  if (!file) return;
  request('PATCH', `/api/files/${file.id}`, { json: { name: `renamed-${unique}.bin` } }, [200], 'PATCH /api/files/:id');
  request('PATCH', `/api/files/${file.id}/move`, { json: { folder_id: a } }, [200], 'PATCH /api/files/:id/move');
  download(file.id, file.sha256);
  request('DELETE', `/api/files/${file.id}`, null, [204], 'DELETE /api/files/:id');
  let entry = trashEntry(file.id, 'target');
  request('POST', `/api/trash/${entry}/restore`, null, [204], 'POST /api/trash/:id/restore');
  purgeFile(file.id, 'target');
  request('DELETE', `/api/folders/${b}`, null, [204], 'DELETE /api/folders/:id');
  entry = trashEntry(b, 'target', 'folder');
  request('POST', `/api/trash/${entry}/restore`, null, [204], 'POST /api/trash/:id/restore');
  request('DELETE', `/api/folders/${b}`, null, [204], 'DELETE /api/folders/:id');
  request('DELETE', `/api/trash/${trashEntry(b, 'target', 'folder')}`, null, [204], 'DELETE /api/trash/:id');
}

export default function () {
  if (testCase === 'plaintext') {
    request('GET', '/benchmark/plaintext', null, [200], 'GET /benchmark/plaintext', 'target', r => r.body === 'Hello, World!');
    return;
  }
  if (testCase === 'json') {
    request('GET', '/benchmark/json', null, [200], 'GET /benchmark/json', 'target', r => r.json('message') === 'Hello, World!');
    return;
  }
  if (testCase === 'echo') {
    request('POST', '/benchmark/echo', echoBody, [200], 'POST /benchmark/echo', 'target', r => r.body === echoBody);
    return;
  }
  if (testCase === 'livez' || testCase === 'readyz') {
    const path = `/${testCase}`;
    request('GET', path, null, [200], `GET ${path}`);
    return;
  }
  if (testCase === 'login' || testCase === 'register') selectAccount();
  else authenticate();
  switch (testCase) {
    case 'profile':
      request('GET', '/api/profile', null, [200], 'GET /api/profile', 'target', r => r.json('username') === account.username);
      break;
    case 'quota': request('GET', '/api/quota', null, [200], 'GET /api/quota', 'target', r => r.json('used') >= 0 && r.json('remaining') >= 0); break;
    case 'root': request('GET', '/api/folders/root/children', null, [200], 'GET /api/folders/root/children', 'target', r => r.json('root.id') === account.root); break;
    case 'children': request('GET', `/api/folders/${account.folder}/children`, null, [200], 'GET /api/folders/:id/children', 'target', r => r.json('folder.id') === account.folder); break;
    case 'contents': request('GET', `/api/folders/${account.folder}/contents?page_size=100&sort=name`, null, [200], 'GET /api/folders/:id/contents', 'target', r => Array.isArray(r.json('items'))); break;
    case 'search': request('GET', '/api/search?q=seed&type=all&page_size=50', null, [200], 'GET /api/search', 'target', r => Array.isArray(r.json('items'))); break;
    case 'trash': request('GET', '/api/trash?page_size=50', null, [200], 'GET /api/trash', 'target', r => Array.isArray(r.json('items'))); break;
    case 'login':
      request('POST', '/login', { json: { username: account.username, password: fixtures.password } }, [200], 'POST /login', 'target', r => Boolean(r.json('access_token')));
      break;
    case 'register':
      request('POST', '/register', { json: { username: `perf_${runId}_${__VU}_${__ITER}`, password: fixtures.password } }, [200], 'POST /register', 'target', r => Boolean(r.json('access_token')));
      break;
    case 'download': download(account.file, account.sha256); break;
    case 'upload': {
      const file = upload();
      if (file) purgeFile(file.id, 'cleanup');
      break;
    }
    case 'workflow': workflow(); break;
    case 'login_mixed':
      if (__ITER % 5 === 0) {
        request('POST', '/login', { json: { username: account.username, password: fixtures.password } }, [200], 'POST /login');
      } else {
        request('GET', '/api/profile', null, [200], 'GET /api/profile');
      }
      break;
    case 'mixed': {
      if (virtualUsers > 1 && __VU <= uploadActors) {
        const file = upload();
        if (file) purgeFile(file.id, 'cleanup');
        break;
      }
      if (!mixedDownload) {
        const seed = request('GET', '/api/search?q=seed-file&type=file', null, [200],
          'GET /api/search', 'preparation').json('items').find(item => item.name === 'seed-file.bin');
        if (!seed) execution.test.abort('Mixed load seed file is missing');
        mixedDownload = seed;
      }
      const operation = __ITER % 10;
      if (virtualUsers === 1 && operation === 9) { const file = upload(); if (file) purgeFile(file.id, 'cleanup'); }
      else if (operation < 2) download(mixedDownload.id, mixedDownload.sha256);
      else if (operation < 4) request('GET', '/api/search?q=seed', null, [200], 'GET /api/search');
      else if (operation < 7) request('GET', `/api/folders/${account.folder}/contents`, null, [200], 'GET /api/folders/:id/contents');
      else request('GET', '/api/profile', null, [200], 'GET /api/profile');
      break;
    }
    default: throw new Error(`Unknown CASE ${testCase}`);
  }
}

export function handleSummary(data) {
  const routes = {};
  for (const route of routeNames) {
    const count = data.metrics[`target_requests{route:${route}}`];
    if (!count || !count.values.count) continue;
    routes[route] = {
      requests: count.values.count,
      successes: data.metrics[`target_successes{route:${route}}`]?.values.count || 0,
      duration_ms: data.metrics[`target_duration_ms{route:${route}}`]?.values,
      complete_ms: data.metrics[`target_complete_ms{route:${route}}`]?.values,
      check_rate: data.metrics[`target_checks{route:${route}}`]?.values.rate,
    };
  }
  const output = {
    script_sha256: scriptHash,
    upload_actor_vus: testCase === 'mixed' && virtualUsers > 1 ? uploadActors : 0,
    case: testCase, vus: Number(__ENV.VUS || 10), rate: __ENV.RATE || null,
    short_connections: __ENV.SHORT_CONNECTIONS === '1',
    content_mode: __ENV.CONTENT_MODE || 'duplicate', payload_bytes: fileData.length,
    routes, summary: data,
  };
  return {
    [__ENV.SUMMARY_FILE || 'summary.json']: JSON.stringify(output, null, 2),
    stdout: JSON.stringify({ case: testCase, requests: data.metrics.target_requests?.values.count,
      check_rate: data.metrics.target_checks?.values.rate, routes: Object.keys(routes).length }) + '\n',
  };
}
