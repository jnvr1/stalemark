# workspace filter

```bash
npm run build -w app
pnpm --filter app run deploy
npm run build --workspace=packages/app
pnpm --filter nope run build
npm start -w app
npm run build --workspaces
pnpm -r run build
npm run build --if-present
pnpm --filter @acme/lib run deploy
```
