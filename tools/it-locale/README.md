# Locale italiano (fork Nebello)

Due script, uno per lavoro:

- `add-italian-locale.py <repo-root>` — registra `it` nei sei punti che
  servono: tipo e elenco in `packages/core/i18n/types.ts`, `supportedLanguages`
  in `server/internal/handler/auth.go`, import e mappa in
  `packages/views/locales/index.ts`, etichetta `preferences.language.italian`
  in tutti i locale, voce nel selettore. Crea `it/` come copia di `en/` se
  manca. Idempotente. Alla fine ricontrolla la parita' delle chiavi con la
  stessa normalizzazione dei plurali di `parity.test.ts`.

- `translate_it.py <repo-root> [namespace ...]` — rigenera `packages/views/locales/it/`
  camminando la struttura di `en/` e sostituendo i valori dalla mappa `T`.
  Le chiavi le detta sempre `en/`, quindi la parita' vale per costruzione e le
  stringhe non ancora tradotte restano inglesi (fallback i18next). I segnaposto
  `{{...}}` sono verificati: se una traduzione li perde, lo script si ferma.

## Dopo ogni rebase su una release upstream

    python3 tools/it-locale/add-italian-locale.py .
    python3 tools/it-locale/translate_it.py .

Il primo riapplica la registrazione, il secondo riallinea `it/` alle chiavi
nuove di `en/`. Le chiavi aggiunte a monte compaiono in inglese finche' non
entrano nella mappa: nessun branch rotto, nessun test rosso.

## Glossario

Invariabili in inglese, perche' sono entita' presenti in URL, API, CLI e nei
prompt degli agent: `issue`, `task`, `skill`, `runtime`, `autopilot`, `inbox`,
`chat`. Italianizzate le parole comuni: agenti, squadre, progetti, consumi,
impostazioni, membri, etichette. Regola ereditata da
`apps/docs/content/docs/developers/conventions.mdx`.
