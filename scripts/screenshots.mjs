// Full-page screenshots of the demo via headless Chrome and the DevTools protocol.
// Called by scripts/screenshots, which starts the app and converts the PNGs.
// Environment: BASE (app URL), OUT (target directory), CHROME (browser binary).
import { spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const BASE = process.env.BASE || 'http://localhost:18095';
const OUT = process.env.OUT;
const WIDTH = 760;
const CHROME = process.env.CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
mkdirSync(OUT, { recursive: true });

const profile = mkdtempSync(join(tmpdir(), 'shot-profile-'));
const chrome = spawn(CHROME, ['--headless=new', '--remote-debugging-port=9333', `--user-data-dir=${profile}`, '--hide-scrollbars', '--no-first-run', 'about:blank'], { stdio: 'ignore' });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function pageWs() {
  for (let i = 0; i < 50; i++) {
    try {
      const list = await (await fetch('http://127.0.0.1:9333/json/list')).json();
      const p = list.find((t) => t.type === 'page');
      if (p) return p.webSocketDebuggerUrl;
    } catch {}
    await sleep(200);
  }
  throw new Error('chrome did not start');
}

const ws = new WebSocket(await pageWs());
await new Promise((r) => (ws.onopen = r));
let nextId = 0;
const pending = new Map();
const waiters = [];
ws.onmessage = (ev) => {
  const m = JSON.parse(ev.data);
  if (m.id && pending.has(m.id)) {
    const { res, rej } = pending.get(m.id);
    pending.delete(m.id);
    m.error ? rej(new Error(JSON.stringify(m.error))) : res(m.result);
  } else if (m.method) {
    for (const w of [...waiters]) if (w.method === m.method) { waiters.splice(waiters.indexOf(w), 1); w.res(); }
  }
};
const send = (method, params = {}) => new Promise((res, rej) => { const id = ++nextId; pending.set(id, { res, rej }); ws.send(JSON.stringify({ id, method, params })); });
const event = (method) => new Promise((res) => waiters.push({ method, res }));
const evalJs = async (expression) => (await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })).result.value;

await send('Page.enable');
await send('Emulation.setDeviceMetricsOverride', { width: WIDTH, height: 1000, deviceScaleFactor: 2, mobile: false });

async function go(path) {
  const loaded = event('Page.loadEventFired');
  await send('Page.navigate', { url: BASE + path });
  await loaded;
  await sleep(500);
}

async function shot(name, path, { js, theme = 'light', maxHeight } = {}) {
  await evalJs(`localStorage.setItem('theme','${theme}')`);
  await go(path);
  if (js) { await evalJs(js); await sleep(400); }
  const h = Math.min(await evalJs('Math.ceil(document.documentElement.scrollHeight)'), maxHeight || Infinity);
  const r = await send('Page.captureScreenshot', { format: 'png', captureBeyondViewport: true, clip: { x: 0, y: 0, width: WIDTH, height: h, scale: 1 } });
  writeFileSync(join(OUT, `${name}.png`), Buffer.from(r.data, 'base64'));
  console.log(`${name}: ${WIDTH}x${h}`);
}

const click = (sel, text) => `(() => { const el = [...document.querySelectorAll('${sel}')].find(e => !${JSON.stringify(text)} || e.textContent.trim() === ${JSON.stringify(text)}); el.click(); })()`;

// Demo login: the cookie lands on the origin, every later page is the demo database.
await go('/dashboard');
await evalJs(`fetch('/login',{method:'POST',headers:{'Content-Type':'application/x-www-form-urlencoded'},body:new URLSearchParams({password:'demo',next:'/dashboard'}),credentials:'include',redirect:'manual'}).then(r=>r.type)`);

const firstId = (listPath, prefix) => evalJs(`fetch('${listPath}').then(r=>r.text()).then(t=>{const m=t.match(/href="[^"]*\\/${prefix}\\/(\\d+)"/);return m&&m[1]})`);
await go('/ablesungen');
const ablesungId = await firstId('/ablesungen', 'ablesungen');
const fixkostenId = await firstId('/fixkosten', 'fixkosten');

const dash = (extra = '') => `${click('.wohnung-tab', 'Wohnung 1')};${click('[data-modus-btn]', 'Verbrauch')};${extra}`;
await shot('dashboard', '/dashboard', { js: dash() });
await shot('dashboard-fixkosten', '/dashboard', { js: dash(click('[data-modus-btn]', 'Fixkosten')) });
await shot('dashboard-kombiniert', '/dashboard', { js: dash(click('[data-modus-btn]', 'Kombiniert')) });
await shot('dashboard-wohnung2', '/dashboard', { js: dash(click('.wohnung-tab', 'Wohnung 2')) });
await shot('dashboard-pv-anlage', '/dashboard', { js: dash(click('.wohnung-tab', 'PV-Anlage')) });
const hideBanner = `document.querySelector('.demo-banner')?.remove()`;
await shot('hero', '/dashboard', { theme: 'dark', js: dash(click('.wohnung-tab', 'Wohnung 2') + ';' + hideBanner), maxHeight: 1150 });
await shot('dashboard-dunkel', '/dashboard', { theme: 'dark', js: dash() });
await shot('ablesungen', '/ablesungen');
await shot('ablesung-detail', `/ablesungen/${ablesungId}`);
await shot('ablesung-bearbeiten', `/ablesungen/${ablesungId}/bearbeiten`);
await shot('fixkosten', '/fixkosten');
await shot('fixkosten-detail', `/fixkosten/${fixkostenId}`);
await shot('fixkosten-neu', '/fixkosten/neu');
await shot('stammdaten', '/stammdaten');
await shot('abrechnung', '/abrechnung', { maxHeight: 1900 });

ws.close();
chrome.kill();
