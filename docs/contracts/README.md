# Contract generation

The approved source of truth is `spec-package/api/openapi.json`, the four JSON Schemas, and the three SQLite DDL files. Do not edit generated outputs directly.

Generate with Node 22:

```bash
fnm exec --using=22 node docs/contracts/generate.mjs
```

Verify without writing:

```bash
fnm exec --using=22 node docs/contracts/generate.mjs --check
```

The generator writes the same aggregate SHA-256 into Go, TypeScript, and `docs/contracts/contract.sha256`. Schema and migration snapshots are copied byte-for-byte.
