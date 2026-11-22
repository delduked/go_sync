// Execute the actual github-script bodies against a fake GitHub API and real files.
// actionlint validates the surrounding YAML; these tests exercise publication behavior.
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');

const workflow = fs.readFileSync(path.join(__dirname, '../.github/workflows/TagReleaseBuildPush.yaml'), 'utf8');
function scriptFor(stepID) {
  const lines = workflow.split('\n');
  const step = lines.findIndex(line => line.trim() === `id: ${stepID}`);
  assert.ok(step >= 0, `Missing step ${stepID}`);
  const start = lines.findIndex((line, i) => i > step && line.trim() === 'script: |');
  assert.ok(start >= 0, `Missing script for ${stepID}`);
  const indent = lines[start].search(/\S/) + 2;
  let end = start + 1;
  while (end < lines.length && (!lines[end].trim() || lines[end].search(/\S/) >= indent)) end++;
  const source = lines.slice(start + 1, end).map(line => line.slice(indent)).join('\n');
  return new (Object.getPrototypeOf(async function () {}).constructor)(
    'github', 'context', 'core', 'require', 'process', 'Date', source);
}
const tagScript = scriptFor('merge');
const publishScript = scriptFor('publish');
const sha = '1234567890abcdef1234567890abcdef12345678';
const mergedAt = '2026-11-22T00:00:00Z';
const tag = 'v_123456.6b023080';
const releaseName = 'v_123456.20261122';
const binaries = ['gosync-darwin-amd64', 'gosync-darwin-arm64', 'gosync-linux-amd64',
  'gosync-linux-arm64', 'gosync-windows-amd64.exe', 'gosync-windows-arm64.exe'];
const names = [...binaries, 'SHA256SUMS'].sort();
const missing = () => Object.assign(new Error('Not found'), {status: 404});

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'gosync-release-test-'));
  t.after(() => fs.rmSync(root, {recursive: true, force: true}));
  fs.mkdirSync(path.join(root, 'dist'));
  const file = name => path.join(root, 'dist', name);
  const checksums = binaries.map(name => {
    const data = Buffer.from(`fixture executable ${name}\n`);
    fs.writeFileSync(file(name), data);
    return `${crypto.createHash('sha256').update(data).digest('hex')}  ${name}\n`;
  }).join('');
  fs.writeFileSync(file('SHA256SUMS'), checksums);
  const context = {repo: {owner: 'example', repo: 'gosync'}, payload: {
    action: 'closed', repository: {default_branch: 'main'},
    pull_request: {number: 42, merged: true, merged_at: mergedAt, merge_commit_sha: sha, base: {ref: 'main'}},
  }};
  const outputs = {};
  const changes = [];
  const state = {ref: undefined, release: undefined, assets: [], uploadFailure: undefined,
    now: Date.parse('2026-11-22T00:10:00Z')};
  class Clock extends Date {
    constructor(...args) { super(...(args.length ? args : [state.now])); }
    static now() { return state.now; }
  }
  const github = {rest: {git: {
    async getRef() { if (!state.ref) throw missing(); return {data: state.ref}; },
    async createRef(args) {
      changes.push(['tag', args]);
      state.ref = {object: {type: 'commit', sha: args.sha}};
      return {data: state.ref};
    },
  }, repos: {
    async getReleaseByTag() {
      // Exercise the documented published-release lookup: drafts require listing.
      if (!state.release || state.release.draft) throw missing();
      return {data: state.release};
    },
    async listReleases() { return {data: state.release ? [state.release] : []}; },
    async createRelease(args) {
      changes.push(['create', args]);
      state.release = {...args, id: 7, html_url: `https://example.test/releases/${tag}`};
      return {data: state.release};
    },
    async listReleaseAssets() { return {data: [...state.assets]}; },
    async deleteReleaseAsset(args) {
      changes.push(['delete', args]);
      state.assets = state.assets.filter(asset => asset.id !== args.asset_id);
    },
    async uploadReleaseAsset(args) {
      if (args.name === state.uploadFailure) throw new Error('Simulated interrupted upload');
      changes.push(['upload', args]);
      state.assets.push({name: args.name, id: changes.length, state: 'uploaded', size: args.data.length});
      return {data: state.assets.at(-1)};
    },
    async updateRelease(args) {
      assert.deepEqual(state.assets.map(asset => asset.name).sort(), names);
      changes.push(['publish', args]);
      Object.assign(state.release, args);
      return {data: state.release};
    },
  }}, async paginate(method, args) { return (await method(args)).data; }};
  const core = {setOutput(name, value) { outputs[name] = value; }, notice() {}};
  const localRequire = name => name === 'node:fs' ? {
    readdirSync: directory => fs.readdirSync(path.join(root, directory)),
    lstatSync: filename => fs.lstatSync(path.join(root, filename)),
    readFileSync: filename => fs.readFileSync(path.join(root, filename)),
  } : require(name);
  const environment = {env: {RELEASE_TAG: tag, RELEASE_NAME: releaseName, RELEASE_SHA: sha}};
  return {context, outputs, changes, state, file, checksums, environment, github,
    tag: () => tagScript(github, context, core, localRequire, environment, Clock),
    publish: () => publishScript(github, context, core, localRequire, environment, Clock)};
}

test('main merge gets an exact, stable tag and a release decision', async t => {
  const f = fixture(t);
  await f.tag();
  assert.deepEqual(f.outputs, {tag, tagname: releaseName, sha, publish: 'true'});
  assert.equal(f.changes[0][1].sha, sha);
  assert.equal(f.changes[0][1].ref, `refs/tags/${tag}`);
  f.state.now = Date.parse('2026-12-15T12:34:56Z');
  await f.tag();
  assert.equal(f.changes.length, 1, 'rerun must reuse the same tag');
});

test('non-default merge is tagged but cannot be published', async t => {
  const f = fixture(t);
  f.context.payload.pull_request.base.ref = 'feature/storage';
  await f.tag();
  assert.equal(f.outputs.publish, 'false');
  assert.equal(f.changes.length, 1);
  await assert.rejects(f.publish(), /default-branch merge/);
  assert.equal(f.changes.length, 1);
});

test('renaming the default branch to master retains release behavior', async t => {
  const f = fixture(t);
  f.context.payload.repository.default_branch = 'master';
  f.context.payload.pull_request.base.ref = 'master';
  await f.tag();
  assert.equal(f.outputs.publish, 'true');
  await f.publish();
  assert.equal(f.state.release.draft, false);
});

test('closed unmerged PRs and other activity types make no changes', async t => {
  for (const update of [{merged: false}, {action: 'opened'}]) {
    const f = fixture(t);
    if ('merged' in update) f.context.payload.pull_request.merged = update.merged;
    if ('action' in update) f.context.payload.action = update.action;
    await f.tag();
    await assert.rejects(f.publish(), /default-branch merge/);
    assert.equal(f.changes.length, 0);
  }
});

test('invalid merge identity cannot create a tag', async t => {
  for (const change of [{merge_commit_sha: 'main'}, {number: -1}, {number: '42'},
    {merged_at: undefined}, {merged_at: null}, {merged_at: 'not a timestamp'},
    {merged_at: 1795305600}, {merged_at: '1969-12-31T23:59:59Z'}]) {
    const f = fixture(t);
    Object.assign(f.context.payload.pull_request, change);
    await assert.rejects(f.tag(), /invalid merged PR identity/);
    assert.equal(f.changes.length, 0);
  }
});

test('hex timestamps and UTC release dates stay correct across midnight', async t => {
  for (const [time, version, name] of [
    ['2026-11-21T23:59:59Z', 'v_123456.6b02307f', 'v_123456.20261121'],
    ['2026-11-22T00:00:01Z', 'v_123456.6b023081', 'v_123456.20261122'],
  ]) {
    const f = fixture(t);
    f.context.payload.pull_request.merged_at = time;
    f.state.now = Date.parse('2030-01-01T12:00:00Z');
    await f.tag();
    assert.equal(f.outputs.tag, version);
    assert.equal(f.outputs.tagname, name);
    f.environment.env.RELEASE_TAG = version;
    f.environment.env.RELEASE_NAME = name;
    await f.publish();
    const creation = f.changes.find(([action]) => action === 'create')[1];
    assert.equal(creation.tag_name, version);
    assert.equal(creation.name, name);
  }
});

test('publication rejects tags or display names from a different merge time', async t => {
  for (const override of [
    {RELEASE_TAG: 'merge-42-1234567890ab'},
    {RELEASE_TAG: 'v_123456.6b023081'},
    {RELEASE_NAME: 'v_123456.20261121'},
    {RELEASE_NAME: `GoSync ${tag}`},
  ]) {
    const f = fixture(t);
    await f.tag();
    Object.assign(f.environment.env, override);
    await assert.rejects(f.publish(), /default-branch merge|Release name does not match/);
    assert.deepEqual(f.changes.map(([action]) => action), ['tag']);
  }
});

test('existing tags are never moved, even after a failed or repeated run', async t => {
  const f = fixture(t);
  f.state.ref = {object: {type: 'commit', sha: 'f'.repeat(40)}};
  await assert.rejects(f.tag(), /Refusing to move/);
  await assert.rejects(f.publish(), /no longer points/);
  assert.equal(f.changes.length, 0);
});

test('API permission failures are surfaced instead of treated as missing tags', async t => {
  const f = fixture(t);
  f.github.rest.git.getRef = async () => { throw Object.assign(new Error('Forbidden'), {status: 403}); };
  await assert.rejects(f.tag(), /Forbidden/);
  assert.equal(f.changes.length, 0);
});

test('six verified binaries and checksums upload before the draft is published', async t => {
  const f = fixture(t);
  await f.tag();
  await f.publish();
  const creation = f.changes.find(([action]) => action === 'create')[1];
  assert.equal(creation.tag_name, tag);
  assert.equal(creation.name, releaseName);
  assert.equal(creation.target_commitish, sha);
  assert.equal(creation.draft, true);
  assert.equal(creation.generate_release_notes, true);
  assert.deepEqual(f.changes.filter(([action]) => action === 'upload').map(([, args]) => args.name).sort(), names);
  assert.equal(f.changes.at(-1)[0], 'publish');
  const mutations = f.changes.length;
  await f.publish();
  assert.equal(f.changes.length, mutations, 'published assets must not be overwritten on rerun');
});

test('an interrupted upload stays a draft and a rerun completes it', async t => {
  const f = fixture(t);
  await f.tag();
  f.state.uploadFailure = binaries[2];
  await assert.rejects(f.publish(), /interrupted upload/);
  assert.equal(f.state.release.draft, true);
  assert.ok(f.state.assets.length > 0);
  assert.ok(!f.changes.some(([action]) => action === 'publish'));
  f.state.uploadFailure = undefined;
  await f.publish();
  assert.equal(f.state.release.draft, false);
  assert.deepEqual(f.state.assets.map(asset => asset.name).sort(), names);
});

test('corrupt, missing, extra, and non-regular downloads cannot create a release', async t => {
  const mutations = [
    f => fs.appendFileSync(f.file(binaries[0]), 'corrupted'),
    f => fs.unlinkSync(f.file(binaries[1])),
    f => fs.writeFileSync(f.file('unexpected.sh'), 'bad'),
    f => { fs.unlinkSync(f.file(binaries[0])); fs.symlinkSync(f.file(binaries[1]), f.file(binaries[0])); },
    f => fs.writeFileSync(f.file(binaries[0]), ''),
    f => fs.appendFileSync(f.file('SHA256SUMS'), f.checksums.split('\n')[0] + '\n'),
    f => fs.writeFileSync(f.file('SHA256SUMS'), `${'a'.repeat(64)}  ../../private\n`),
  ];
  for (const mutate of mutations) {
    const f = fixture(t);
    await f.tag();
    mutate(f);
    await assert.rejects(f.publish(), /Checksum mismatch|exactly six|regular file|Empty release|checksum entry/);
    assert.deepEqual(f.changes.map(([action]) => action), ['tag']);
  }
});

test('incomplete public releases and unexpected draft assets are left untouched', async t => {
  for (const draft of [true, false]) {
    const f = fixture(t);
    await f.tag();
    f.state.release = {id: 7, tag_name: tag, draft};
    f.state.assets = [{id: 10, name: 'unexpected', state: 'uploaded', size: 12}];
    await assert.rejects(f.publish(), /incomplete|unexpected assets/);
    assert.deepEqual(f.changes.map(([action]) => action), ['tag']);
  }
});
