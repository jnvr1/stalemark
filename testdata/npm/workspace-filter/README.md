# workspace filter

```bash
npm run build -w app
pnpm --filter app run deploy
npm run build --workspace=packages/app
pnpm --filter nope run build
```
