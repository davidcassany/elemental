#!/usr/bin/env node

const fs = require("fs");
const path = require("path");

const target = process.argv[2] || "docs/viz/elemental-config-viz.html";
const htmlPath = path.resolve(process.cwd(), target);
const html = fs.readFileSync(htmlPath, "utf8");
const match = html.match(/<script>([\s\S]*)<\/script>\s*<\/body>/);

if (!match) {
  throw new Error(`inline script not found in ${target}`);
}

new Function(match[1]);
console.log(`inline script syntax ok: ${target}`);
