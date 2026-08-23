# Signal Ledger frontend

React + TypeScript + Vite portfolio shell for the Go microservices project. The access token is kept in memory; refresh uses the Gateway HttpOnly cookie path and is never persisted to local storage. Role checks are navigation UX only—the Gateway and services remain the security boundary.

```powershell
npm install
npm run lint
npm test
npm run build
npm run dev
```
