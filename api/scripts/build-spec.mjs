#!/usr/bin/env node
// Auto-discovers every services/*.yaml and joins them into openapi.yaml.
// No need to maintain a service list as you add domains — just drop a new
// services/<domain>.yaml and rebuild.
//
// Usage: node scripts/build-spec.mjs [config/servers-<env>.yaml] [-o openapi.yaml]
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
import YAML from "yaml";

const serverConfig = process.argv[2] || "config/servers-local.yaml";
const output = process.argv[4] || "openapi.yaml";
const servicesDir = "services";

// Shared component specs (if present) must be joined before the specs that
// $ref them. Everything else is alphabetical for deterministic output.
const priority = ["global.yaml", "shared.yaml"];
const rank = (f) => {
  const i = priority.indexOf(f);
  return i === -1 ? priority.length : i;
};

const files = readdirSync(servicesDir)
  .filter((f) => f.endsWith(".yaml") || f.endsWith(".yml"))
  .sort((a, b) => rank(a) - rank(b) || a.localeCompare(b))
  .map((f) => join(servicesDir, f));

if (files.length === 0) {
  console.error(`No service specs found in ${servicesDir}/`);
  process.exit(1);
}

console.log(`Joining ${files.length} spec(s) -> ${output}`);
execFileSync(
  "npx",
  ["redocly", "join", serverConfig, ...files, "-o", output],
  { stdio: "inherit" },
);

// Post-process: `redocly join` pushes an (empty) `servers: []` onto every path
// and operation to record provenance. That trips up some OpenAPI consumers
// (e.g. swagger_parser casts the node to a map). Strip the empty arrays and set
// the top-level servers from the chosen config.
const doc = YAML.parse(readFileSync(output, "utf8"));
const config = YAML.parse(readFileSync(serverConfig, "utf8"));
if (config.servers) doc.servers = config.servers;

const methods = ["get", "post", "put", "delete", "patch", "options", "head", "trace"];
const isEmpty = (s) => Array.isArray(s) && s.length === 0;
for (const pathItem of Object.values(doc.paths ?? {})) {
  if (isEmpty(pathItem.servers)) delete pathItem.servers;
  for (const m of methods) {
    if (pathItem[m] && isEmpty(pathItem[m].servers)) delete pathItem[m].servers;
  }
}
writeFileSync(output, YAML.stringify(doc));
console.log("Post-processing: set top-level servers, removed empty per-path servers.");
