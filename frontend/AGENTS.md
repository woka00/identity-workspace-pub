# Frontend Codex guide

Scope: `frontend/**`. Read `ARCHITECTURE.md` only for changes that alter frontend layer boundaries or move responsibilities between layers.

## Architecture map

```text
presentation (React)
  -> application (pure product rules)
  -> domain (API-independent models)
  -> infrastructure (HTTP/browser/export adapters)
```

More precisely, presentation may depend on application/domain/infrastructure; application depends on domain; infrastructure depends on domain; domain depends on no project layer. `scripts/check-architecture.mjs` enforces this direction.

## High-value file map

- App shell, auth gate, main navigation, legacy/main screens: `src/App.tsx`
- Shared API/domain DTOs: `src/domain/models.ts`
- Backend HTTP adapter: `src/infrastructure/http/apiClient.ts`
- Task rich description/subtasks: `src/presentation/tasks/TaskDescriptionEditor.tsx`
- Time tracker: `src/presentation/time-tracker/TimeTrackerPage.tsx`
- Time statistics: `src/presentation/time-tracker/TimeStatisticsPage.tsx`
- Food admin: `src/presentation/nutrition/AdminFoodsPage.tsx`
- Barcode sheet/scanning: `src/presentation/nutrition/BarcodeScannerSheet.tsx`, `src/infrastructure/browser/barcodeScanner.ts`, `barcodeCamera.ts`
- OAuth callback helpers: `src/infrastructure/browser/oauthCallbacks.ts`
- Date/product rules: `src/application/dateKeys.ts`, `nutritionGoals.ts`
- Calorie PDF export: `src/infrastructure/export/calorieReport.ts`
- Global/component styles: `src/styles.css`
- PWA entry files: `public/sw.js`, `public/manifest.webmanifest`

## Critical token-saving rule

`src/App.tsx` (~6.1k lines) and `src/styles.css` (~7.5k lines) are large. Never read either file top-to-bottom for a normal task.

For `App.tsx`:

1. search the component/state/text first, e.g. `rg -n 'function TasksPage|trackerModal|FatSecret' src/App.tsx`;
2. inspect only the component/function range around matches;
3. trace imported helpers/models only if the change requires them.

For `styles.css`:

1. identify the JSX class name first;
2. `rg -n '^\.className|className-fragment' src/styles.css`;
3. inspect the local selector block and relevant nearby media-query overrides only.

Do not ingest all CSS merely to make one visual adjustment.

## UI ownership

- Persisted data comes from backend APIs; transient animation/edit/navigation state may live in React.
- The time-tracker one-second tick is display-only. Server/PostgreSQL values remain authoritative and are reconciled on the existing lifecycle triggers.
- Access control, catalog visibility, and authorization decisions belong to backend, not React.
- Backend URLs stay relative to preserve same-origin cookies in browser/Safari/PWA.
- OAuth tokens/secrets never belong in frontend code or browser storage.
- Preserve existing per-user storage-key scoping. Do not rename storage keys unless migration/compatibility is explicitly handled.

## Feature routing inside App.tsx

Search these component names instead of browsing the whole file:

- `AuthenticatedApp`: top-level authenticated state/loading/navigation
- `TrackerPreview`: tracker cards, calories, water, weight, GitHub, custom trackers
- `TrackerStatistics`: legacy tracker statistics
- `TasksPage` / `TaskRow` / `TaskEditor`: tasks, categories, recurrence, editing
- `ProjectsPage` / `GoalEditor`: projects/goals
- `ProfilePage` / `ProfileEditor` / `SignatureEditor`: profile/integrations/signature
- `FatSecretFoodPicker` / `FoodResultRow`: food search and entries
- `IDCard`: portfolio/identity card UI

Time-tracker UI is already split into `src/presentation/time-tracker/*`; modify those files before considering new time-tracker code in `App.tsx`.

## Styling conventions

- Reuse existing visual language, spacing, variables, controls, and mobile breakpoints before adding a parallel design system.
- Check both base selector and mobile overrides for changed classes.
- Preserve mobile-first touch behavior and existing iOS focus/keyboard accommodations.
- Respect `prefers-reduced-motion` for new nonessential motion.
- Avoid global selectors when a component-scoped class can express the change.
- Do not edit `dist`; it is a build artifact.

## API changes

When backend response/input shapes change, normally inspect only:

```text
src/domain/models.ts
src/infrastructure/http/apiClient.ts
consumer component(s)
```

Keep fetch/request details in the HTTP adapter when possible instead of scattering direct requests through presentation code.

## PWA/browser constraints

- `public/sw.js` and `public/manifest.webmanifest` are public entry points independent of the bundled application.
- Service worker is used for Web Push/opening the app; do not assume a general offline-data cache exists.
- Browser-only APIs/camera/OAuth glue belong under `src/infrastructure/browser`.

## Validation

From `frontend/`:

```bash
npm run check
npm run build
```

`npm run check` runs architecture and PWA checks. Use `npm run build` for TypeScript/Vite validation when source behavior/UI changes. Do not run `npm ci` just to inspect or make a small change when dependencies are already present; use the repository/CI commands when dependency integrity itself is relevant.

## Frontend completion checks

- search for all uses before renaming a model/property/storage key;
- verify mobile CSS overrides for changed UI;
- keep persisted/server state authoritative;
- keep API calls same-origin and secrets backend-only;
- ensure `npm run check` and, for source changes, `npm run build` pass when environment permits.
