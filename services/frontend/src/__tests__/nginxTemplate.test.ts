import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

describe('nginx static-only config', () => {
  const configPath = resolve(process.cwd(), 'nginx.conf');
  const conf = readFileSync(configPath, 'utf8');

  it('serves static files with SPA fallback', () => {
    expect(conf).toContain('location / {');
    expect(conf).toContain('try_files $uri /index.html;');
  });

  it('exposes /healthz and does not include /api reverse proxy rules', () => {
    expect(conf).toContain('location = /healthz {');
    expect(conf).toContain('return 200;');
    expect(conf).not.toContain('location /api');
    expect(conf).not.toContain('proxy_pass');
  });
});
