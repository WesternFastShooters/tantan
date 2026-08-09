# Folo 基线安全导入

## 1. 目标

把 Folo commit `3846c90b67da351b6017cd4fe9d0992b8077224e` 的受版本控制文件导入 Tantan 根目录，同时保留现有 PRD、原型、三份规格和 `spec-package/**`。导入不得修改 Folo 源仓库，不得覆盖任何已有 Tantan 文件。

## 2. 开工前的硬门禁

1. 在 Tantan 根运行 `bash spec-package/scripts/validate-package.sh`。
2. 读取 `git -C /Users/mingrui/Project/Folo status --short` 和 `git -C /Users/mingrui/Project/Folo rev-parse HEAD`。要求工作树无更改且 HEAD 精确等于锁定 commit；不符时停止，不在 Folo 中 checkout/reset。
3. 记录 Tantan 根现有文件和 SHA-256。规格包 `manifest.json` 已包含所有权威输入哈希。
4. Tantan 根如尚无 Git，先 `git init`，只精确 `git add` 三份规格、PRD、原型和 `spec-package/**`，创建规格基线 commit；不添加 `.DS_Store`。

## 3. 导入算法

1. 用 `mktemp -d` 创建专用临时目录。
2. 使用 `git -C /Users/mingrui/Project/Folo archive 3846c90b67da351b6017cd4fe9d0992b8077224e` 解包到临时目录。不复制 Folo `.git` 和未跟踪文件。
3. 遍历归档中的每个相对路径；如果 Tantan 根已有同路径，导入整体失败并列出冲突，不得覆盖或用 `--force`。
4. 只在冲突集为空时，将临时归档同步到 Tantan 根。使用“ignore existing + checksum”模式作为第二道保护，不使用 `--delete`。
5. 导入后立即重跑整包校验，确认权威输入哈希未变。
6. 检查根 `package.json`、`.nvmrc`、lockfile 与 SDK 版本；记录未改造的 typecheck/Vitest/Web E2E 基线。
7. 将导入作为独立 commit；不与任何功能删除或产品改动混合。

## 4. 导入后必须成立

- `/Users/mingrui/Project/Folo` 的 status 和 HEAD 与导入前相同。
- Tantan 的 PRD、原型、规格和规格包 SHA-256 与 `manifest.json` 相同。
- Tantan 中存在 Folo 根 `package.json`、`pnpm-lock.yaml`、`apps/desktop/**`、`packages/internal/**`。
- `apps/mobile/**` 可以作为 monorepo 基线存在，但一期任务 diff 必须为零。
- 导入 commit 之后才能领取 `TASK-CONTRACT`；不允许在导入 commit 中顺手移除付费功能。
