#!/usr/bin/env python3
"""Registra la lingua italiana in Multica, senza tradurre nulla.

`it/` nasce come copia di `en/`: le chiavi esistono tutte da subito, quindi
`packages/views/locales/parity.test.ts` resta verde mentre si traduce a
scaglioni. I valori non ancora tradotti si leggono in inglese, che e'
esattamente il comportamento di fallback di i18next — nessuna schermata rotta.

Idempotente: rilanciarlo non duplica nulla.

Uso: add-italian-locale.py <repo-root>
"""
import json
import pathlib
import re
import shutil
import sys

ROOT = pathlib.Path(sys.argv[1]).resolve()
LOC = ROOT / "packages/views/locales"
changed = []


def edit(path: pathlib.Path, fn):
    p = ROOT / path
    before = p.read_text()
    after = fn(before)
    if after != before:
        p.write_text(after)
        changed.append(str(path))


# 1. it/ come copia di en/
if not (LOC / "it").exists():
    shutil.copytree(LOC / "en", LOC / "it")
    changed.append("packages/views/locales/it/ (copia di en/)")

# 2. il tipo e l'elenco delle lingue supportate
def patch_types(s: str) -> str:
    s = s.replace(
        'export type SupportedLocale = "en" | "zh-Hans" | "ko" | "ja";',
        'export type SupportedLocale = "en" | "zh-Hans" | "ko" | "ja" | "it";',
    )
    return s.replace(
        'export const SUPPORTED_LOCALES: SupportedLocale[] = ["en", "zh-Hans", "ko", "ja"];',
        'export const SUPPORTED_LOCALES: SupportedLocale[] = ["en", "zh-Hans", "ko", "ja", "it"];',
    )


edit(pathlib.Path("packages/core/i18n/types.ts"), patch_types)

# 3. il server valida `language`: senza questo, PATCH /api/me risponde 400
#    "unsupported language" e la scelta non si propaga agli altri dispositivi.
def patch_auth_go(s: str) -> str:
    if '"it":' in s:
        return s
    return s.replace(
        '\t"ja":      {},\n}',
        '\t"ja":      {},\n\t"it":      {},\n}',
    )


edit(pathlib.Path("server/internal/handler/auth.go"), patch_auth_go)

# 4. import + blocco `it` nella mappa RESOURCES
NS = [
    ("common", "Common"), ("auth", "Auth"), ("settings", "Settings"), ("issues", "Issues"),
    ("agents", "Agents"), ("editor", "Editor"), ("onboarding", "Onboarding"), ("invite", "Invite"),
    ("labels", "Labels"), ("members", "Members"), ("my-issues", "MyIssues"), ("search", "Search"),
    ("inbox", "Inbox"), ("workspace", "Workspace"), ("projects", "Projects"),
    ("autopilots", "Autopilots"), ("skills", "Skills"), ("chat", "Chat"), ("modals", "Modals"),
    ("runtimes", "Runtimes"), ("layout", "Layout"), ("usage", "Usage"), ("ui", "Ui"),
    ("squads", "Squads"), ("billing", "Billing"),
]


def patch_index(s: str) -> str:
    if "itCommon" in s:
        return s
    imports = "".join(f'import it{cam} from "./it/{ns}.json";\n' for ns, cam in NS)
    # gli import dell'ultima lingua (ja) finiscono prima della prima riga vuota
    last_ja = s.rindex('import ja')
    end = s.index("\n", last_ja) + 1
    s = s[:end] + imports + s[end:]

    body = "".join(
        f'    {ns if ns.isidentifier() else chr(34) + ns + chr(34)}: it{cam},\n' for ns, cam in NS
    )
    block = "  it: {\n" + body + "  },\n"
    # inserisce il blocco prima della chiusura dell'oggetto RESOURCES
    return s[: s.rindex("};")] + block + s[s.rindex("};") :]


edit(pathlib.Path("packages/views/locales/index.ts"), patch_index)

# 5. etichetta della lingua: serve in TUTTI i locale, il parity test pretende
#    le stesse chiavi ovunque. Gli endonimi restano tali in ogni lingua.
#
#    Inserimento testuale e non round-trip JSON: json.dumps riformatta gli
#    oggetti scritti su una riga sola (`{ "general": "..." }`) ed espande 300
#    righe di rumore, che sul rebase e' debito puro.
def add_italian_label(s: str) -> str:
    if '"italian"' in s:
        return s
    lines = s.splitlines(keepends=True)
    hits = [i for i, l in enumerate(lines) if re.match(r'^\s*"japanese":', l)]
    if len(hits) != 1:
        raise SystemExit(f"attesa una riga \"japanese\", trovate {len(hits)}")
    i = hits[0]
    indent = re.match(r"^(\s*)", lines[i]).group(1)
    lines.insert(i + 1, f'{indent}"italian": "Italiano",\n')
    return "".join(lines)


for loc in ("en", "zh-Hans", "ko", "ja", "it"):
    edit(pathlib.Path(f"packages/views/locales/{loc}/settings.json"), add_italian_label)

# 6. voce nel selettore (lista scritta a mano, non derivata da SUPPORTED_LOCALES)
def patch_switcher(s: str) -> str:
    if 'value: "it"' in s:
        return s
    return s.replace(
        '    { value: "en", label: t(($) => $.preferences.language.english) },',
        '    { value: "en", label: t(($) => $.preferences.language.english) },\n'
        '    { value: "it", label: t(($) => $.preferences.language.italian) },',
    )


edit(pathlib.Path("packages/views/settings/components/preferences-tab.tsx"), patch_switcher)


# 7. mappe per-locale tipizzate `Record<SupportedLocale, ...>`: il compilatore
#    le pretende complete, quindi ognuna va estesa. Sono quattro, e le trova
#    `tsc` non il parity test — per questo conviene costruire l'immagine presto.
def add_html_lang(s: str) -> str:
    if 'it: "it-IT"' in s:
        return s
    return s.replace('  ja: "ja-JP",\n};', '  ja: "ja-JP",\n  it: "it-IT",\n};')


for f in ("apps/web/app/layout.tsx", "apps/desktop/src/renderer/src/App.tsx"):
    edit(pathlib.Path(f), add_html_lang)


# L'onboarding ha contenuti scritti a mano solo in quattro lingue: l'italiano
# ricade sull'inglese invece di mostrare una schermata vuota.
def add_content_lang(s: str) -> str:
    if 'it: "en"' in s:
        return s
    return s.replace('  ja: "ja",\n};', '  ja: "ja",\n  it: "en",\n};')


edit(pathlib.Path("packages/views/onboarding/templates/index.ts"), add_content_lang)


def add_use_case_text(s: str) -> str:
    if "  it: {" in s:
        return s
    it_block = """  it: {
    indexTitle: "Casi d'uso",
    indexSubtitle:
      "Guarda come i team organizzano persone e agenti insieme con Multica.",
    indexMetadataTitle: "Casi d'uso",
    indexMetadataDescription:
      "Guarda come i team mettono persone e agenti a lavorare insieme con Multica.",
    cardReadMore: "Leggi \u2192",
    tableOfContents: "In questa pagina",
  },
"""
    marker = "export const useCaseText: Record<SupportedLocale, UseCaseText> = {\n"
    i = s.index(marker) + len(marker)
    return s[:i] + it_block + s[i:]


edit(pathlib.Path("apps/web/lib/use-cases-i18n.ts"), add_use_case_text)


# 8. la i18n della landing ha il proprio `Locale = SupportedLocale`, quindi il
#    Record delle etichette va completato. NON aggiungo "it" all'array
#    `locales`: il dizionario della landing esiste solo in 4 lingue e
#    `toLandingDictionaryLocale` ricade su "en", quindi offrire l'italiano nel
#    selettore della landing mostrerebbe copy inglese. Meglio non offrirlo.
def add_landing_label(s: str) -> str:
    if 'it: "IT"' in s:
        return s
    return s.replace(
        '  ja: "\\u65e5\\u672c\\u8a9e",\n};',
        '  ja: "\\u65e5\\u672c\\u8a9e",\n  it: "IT",\n};',
    )


edit(pathlib.Path("apps/web/features/landing/i18n/types.ts"), add_landing_label)

print("modificati:")
for c in changed:
    print("  ", c)

# 7. controllo di parita' equivalente a parity.test.ts, senza dipendenze
def leaves(obj, prefix=""):
    """Chiavi foglia con i plurali normalizzati come fa parity.test.ts:
    EN ha `key_one`+`key_other`, le lingue senza distinzione di numero hanno
    solo `key_other`. Senza questa normalizzazione si contano 91 falsi
    mancanti su zh/ko/ja. L'italiano invece ha il plurale, quindi tiene
    entrambe le forme."""
    out = set()
    for k, v in obj.items():
        key = f"{prefix}{k}"
        if isinstance(v, dict):
            out |= leaves(v, key + ".")
        else:
            out.add(re.sub(r"_(one|other)$", "_count", key))
    return out


print("\nparita' chiavi (equivalente a parity.test.ts):")
ok = True
en_ns = sorted(p.name for p in (LOC / "en").glob("*.json"))
for loc in ("zh-Hans", "ko", "ja", "it"):
    loc_ns = sorted(p.name for p in (LOC / loc).glob("*.json"))
    if loc_ns != en_ns:
        ok = False
        print(f"  {loc}: NAMESPACE DIVERSI: {set(en_ns) ^ set(loc_ns)}")
        continue
    missing = extra = 0
    for ns in en_ns:
        e = leaves(json.loads((LOC / "en" / ns).read_text()))
        l = leaves(json.loads((LOC / loc / ns).read_text()))
        missing += len(e - l)
        extra += len(l - e)
    status = "ok" if (missing == 0 and extra == 0) else f"MANCANTI {missing}, IN ECCESSO {extra}"
    if missing or extra:
        ok = False
    print(f"  {loc}: {status}")

tot = sum(len(leaves(json.loads((LOC / 'it' / ns).read_text()))) for ns in en_ns)
print(f"\nstringhe in it/: {tot} (da tradurre, ora identiche a en)")
sys.exit(0 if ok else 1)
