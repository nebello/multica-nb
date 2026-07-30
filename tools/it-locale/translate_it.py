#!/usr/bin/env python3
"""Applica traduzioni italiane camminando la struttura di `en/`.

Perché non scrivere i JSON a mano: con 4200 stringhe su 25 namespace, una
chiave dimenticata o in eccesso fa fallire `parity.test.ts`. Qui la struttura
la detta sempre `en/`, quindi le chiavi combaciano per costruzione e le
stringhe non ancora tradotte restano in inglese (fallback naturale i18next).

Rilanciabile: rigenera `it/` da `en/` + mappa, quindi correggere una
traduzione significa correggere la mappa, non il JSON.

GLOSSARIO (deciso per questo fork, coerente con la doc upstream):
  - invariabili in inglese, perché sono entità che compaiono in URL, API,
    CLI e nei prompt degli agent: issue, task, skill, runtime, autopilot,
    inbox, chat
  - italianizzati, perché sono parole comuni e non entità: agenti, squadre,
    progetti, consumi, impostazioni, membri, etichette
  - i segnaposto {{...}} non si traducono MAI e vanno riportati identici

Uso: translate_it.py <repo-root> [namespace ...]
"""
import json
import pathlib
import re
import sys

ROOT = pathlib.Path(sys.argv[1]).resolve()
ONLY = set(sys.argv[2:])
LOC = ROOT / "packages/views/locales"

T: dict[str, dict[str, str]] = {}

T["layout"] = {
    "nav.inbox": "Inbox",
    "nav.chat": "Chat",
    "nav.my_issues": "Le mie issue",
    "nav.issues": "Issue",
    "nav.projects": "Progetti",
    "nav.autopilots": "Autopilot",
    "nav.agents": "Agenti",
    "nav.squads": "Squadre",
    "nav.usage": "Consumi",
    "nav.runtimes": "Runtime",
    "nav.skills": "Skill",
    "nav.settings": "Impostazioni",
    "tab.issue": "Issue",
    "tab.project": "Progetto",
    "tab.autopilot": "Autopilot",
    "tab.agent": "Agente",
    "tab.member": "Membro",
    "tab.squad": "Squadra",
    "tab.skill": "Skill",
    "tab.machine": "Macchina",
    "tab.runtime": "Runtime",
    "tab.attachment": "Allegato",
    "tab.create_agent": "Crea agente",
    "tab.unknown": "Pagina sconosciuta",
    "help.trigger": "Aiuto",
    "help.docs": "Documentazione",
    "help.changelog": "Novità",
    "help.discord": "Discord",
    "help.feedback": "Feedback",
    "help.server_version": "Versione del server {{version}}",
    "workspace_loader.loading_workspace": "Caricamento workspace…",
    "workspace_loader.loading_named_prefix": "Caricamento",
    "sidebar.unpin_tooltip": "Togli dai fissati",
    "sidebar.workspaces_label": "Workspace",
    "sidebar.create_workspace": "Crea workspace",
    "sidebar.pending_invitations_label": "Inviti in attesa",
    "sidebar.invitation_workspace_fallback": "Workspace",
    "sidebar.invitation_join": "Entra",
    "sidebar.invitation_decline": "Rifiuta",
    "sidebar.log_out": "Esci",
    "sidebar.new_issue": "Nuova issue",
    "sidebar.pinned_label": "Fissati",
    "sidebar.workspace_group": "Workspace",
    "sidebar.configure_group": "Configurazione",
    "sidebar.discord_card.title": "Entra nel nostro Discord",
    "sidebar.discord_card.description": "Scambia due parole col team e con altri che ci costruiscono sopra.",
    "sidebar.discord_card.dismiss": "Chiudi",
}

T["common"] = {
    "avatar_upload.change": "Cambia avatar",
    "avatar_upload.remove": "Rimuovi",
    "avatar_upload.select_image": "Scegli un file immagine.",
    "avatar_upload.updated": "Avatar aggiornato",
    "avatar_upload.failed": "Caricamento dell'avatar non riuscito",
    "avatar_crop.title": "Modifica avatar",
    "avatar_crop.zoom": "Zoom",
    "avatar_crop.zoom_in": "Ingrandisci",
    "avatar_crop.zoom_out": "Riduci",
    "avatar_crop.rotate": "Ruota",
    "avatar_crop.reset": "Ripristina",
    "avatar_crop.cancel": "Annulla",
    "avatar_crop.apply": "Salva",
    "avatar_crop.uploading": "Caricamento…",
    "avatar_crop.loading": "Caricamento…",
    "avatar_crop.load_failed": "Non è stato possibile caricare l'immagine",
    "color_picker.saturation": "Saturazione e luminosità",
    "color_picker.hue": "Tonalità",
    "color_picker.hex": "Colore esadecimale",
    "color_picker.eyedropper": "Preleva un colore dallo schermo",
    "color_picker.presets": "Predefiniti",
    "color_picker.random": "Casuale",
    "save": "Salva",
    "cancel": "Annulla",
    "delete": "Elimina",
    "confirm": "Conferma",
    "loading": "Caricamento...",
    "time.just_now": "adesso",
    "time.minutes_ago": "{{count}} min fa",
    "time.hours_ago": "{{count}} h fa",
    "time.days_ago": "{{count}} g fa",
    "lark_bind.page_title": "Collega il tuo account Lark",
    "lark_bind.redeeming": "Verifica del token di collegamento…",
    "lark_bind.needs_auth_description": "Accedi a Multica per completare il collegamento. Il token nel link lega il tuo account Lark a questo utente Multica, quindi devi prima aver effettuato l'accesso.",
    "lark_bind.sign_in": "Accedi",
    "lark_bind.done_title": "Collegamento fatto.",
    "lark_bind.done_description": "Il prossimo messaggio che scrivi al Bot su Lark andrà direttamente all'agente. Puoi chiudere questa scheda.",
    "lark_bind.error_title": "Collegamento non completato",
    "lark_bind.error_admin_hint": "Se continua a succedere, chiedi all'amministratore del workspace di rimandare il link di collegamento da Lark.",
    "lark_bind.error_missing_token": "Nel link manca il token di collegamento. Chiedi al Bot di mandarne uno nuovo su Lark.",
    "lark_bind.error_expired": "Questo link non è valido o è scaduto (i link valgono 15 minuti). Chiedi al Bot di mandarne uno nuovo.",
    "lark_bind.error_already_bound": "Questo account Lark è già collegato a un altro utente Multica. Per trasferirlo serve prima scollegarlo esplicitamente.",
    "lark_bind.error_not_member": "Hai effettuato l'accesso con un account Multica che non è membro di questo workspace.",
    "lark_bind.error_unknown": "Qualcosa è andato storto. Riprova e, se il problema persiste, contatta l'amministratore del workspace.",
    "slack_bind.page_title": "Collega il tuo account Slack",
    "slack_bind.redeeming": "Collegamento dell'account…",
    "slack_bind.needs_auth_description": "Accedi a Multica per completare il collegamento. Il token nel link lega il tuo account Slack a questo utente Multica, quindi devi prima aver effettuato l'accesso.",
    "slack_bind.sign_in": "Accedi",
    "slack_bind.done_title": "Collegamento fatto.",
    "slack_bind.done_description": "Il prossimo messaggio che scrivi al bot su Slack andrà direttamente all'agente. Puoi chiudere questa scheda.",
    "slack_bind.error_title": "Collegamento non completato",
    "slack_bind.error_admin_hint": "Se continua a succedere, scrivi di nuovo al bot su Slack per ottenere un link fresco.",
    "slack_bind.error_missing_token": "Nel link manca il token. Scrivi di nuovo al bot su Slack per ottenerne uno nuovo.",
    "slack_bind.error_expired": "Questo link non è valido o è scaduto (i link valgono 15 minuti). Scrivi di nuovo al bot per ottenerne uno nuovo.",
    "slack_bind.error_already_bound": "Questo account Slack è già collegato a un altro utente Multica. Per trasferirlo serve prima scollegarlo esplicitamente.",
    "slack_bind.error_not_member": "Hai effettuato l'accesso con un account Multica che non è membro di questo workspace.",
    "slack_bind.error_unknown": "Qualcosa è andato storto. Riprova e, se il problema persiste, contatta l'amministratore del workspace.",
    "expandable_description.expand": "Mostra tutto",
    "expandable_description.collapse": "Mostra meno",
}

T["ui"] = {
    "attach_file": "Allega file",
    "toggle_sidebar": "Mostra o nascondi la barra laterale",
    "pagination_previous": "Vai alla pagina precedente",
    "pagination_next": "Vai alla pagina successiva",
    "copy_code": "Copia il codice",
    "plain_text": "testo semplice",
}

T["labels"] = {
    "remove_label": "Rimuovi l'etichetta {{name}}",
    "resource_picker.empty": "Nessuna etichetta",
    "resource_picker.add": "Aggiungi etichette",
    "resource_picker.search": "Cerca etichette...",
    "resource_picker.no_labels": "Crea prima le etichette nelle Impostazioni",
    "resource_picker.no_results": "Nessuna etichetta corrispondente",
}

T["members"] = {
    "role.owner": "Proprietario",
    "role.admin": "Amministratore",
    "role.member": "Membro",
    "card.unavailable": "Membro non disponibile",
    "card.agents_section": "Agenti ({{count}})",
    "card.detail_link": "Dettaglio →",
    "card.more_agents_one": "e {{count}} altro agente",
    "card.more_agents_other": "e altri {{count}} agenti",
    "detail.workspace_fallback": "Workspace",
    "detail.members_breadcrumb": "Membri",
    "detail.breadcrumb_fallback": "Membro",
    "detail.not_found_title": "Membro non trovato",
    "detail.not_found_description": "Questo membro potrebbe aver lasciato il workspace.",
}

T["workspace"] = {
    "create_form.name_label": "Nome del workspace",
    "create_form.name_placeholder": "Il mio workspace",
    "create_form.url_label": "URL del workspace",
    "create_form.url_placeholder": "il-mio-workspace",
    "create_form.submit": "Crea workspace",
    "create_form.submitting": "Creazione...",
    "create_form.errors.slug_format": "Solo lettere minuscole, numeri e trattini",
    "create_form.errors.slug_taken": "Questo URL di workspace è già in uso.",
    "create_form.errors.slug_reserved": "Questo URL di workspace è riservato e non si può usare.",
    "create_form.errors.slug_conflict_toast": "Scegli un altro URL di workspace",
    "create_form.errors.create_failed": "Creazione del workspace non riuscita",
    "new_page.back": "Indietro",
    "new_page.log_out": "Esci",
    "new_page.title": "Benvenuto in Multica",
    "new_page.description": "Un workspace dove tu e i tuoi compagni di squadra AI lavorate affiancati: prendete issue, lasciate commenti, condividete lo stesso contesto.",
    "new_page.invite_hint": "Potrai invitare i tuoi collaboratori quando il workspace è pronto.",
    "creation_disabled.title": "La creazione di workspace è disattivata",
    "creation_disabled.description": "Questa istanza di Multica non permette di creare nuovi workspace. Chiedi al tuo amministratore un invito a un workspace esistente.",
    "no_access.title": "Workspace non disponibile",
    "no_access.description": "Questo workspace non esiste oppure non hai accesso.",
    "no_access.go_to_workspaces": "Vai ai miei workspace",
    "no_access.sign_in_different": "Accedi con un altro utente",
}


T["inbox"] = {
    "page.title": "Inbox",
    "page.back": "Inbox",
    "menu.mark_all_read": "Segna tutto come letto",
    "menu.archive_all": "Archivia tutto",
    "menu.archive_all_read": "Archivia tutti i letti",
    "menu.archive_completed": "Archivia i completati",
    "list.empty": "Nessuna notifica",
    "list.mark_done_tooltip": "Segna come fatto",
    "list.archive_tooltip": "Archivia",
    "list.time.just_now": "adesso",
    "list.time.minutes": "{{count}} min",
    "list.time.hours": "{{count}} h",
    "list.time.days": "{{count}} g",
    "list.archived_title": "Archiviate",
    "list.archived_empty": "Nessuna notifica archiviata",
    "list.unarchive_tooltip": "Togli dall'archivio",
    "detail.select_prompt": "Scegli una notifica per vederne i dettagli",
    "detail.empty": "La tua inbox è vuota",
    "detail.original_input": "Testo originale",
    "detail.edit_advanced": "Modifica nel modulo avanzato",
    "detail.archive": "Archivia",
    "detail.unarchive": "Togli dall'archivio",
    "types.issue_assigned": "Assegnata",
    "types.issue_subscribed": "Seguita",
    "types.unassigned": "Assegnazione rimossa",
    "types.assignee_changed": "Assegnatario cambiato",
    "types.status_changed": "Stato cambiato",
    "types.priority_changed": "Priorità cambiata",
    "types.start_date_changed": "Data di inizio cambiata",
    "types.due_date_changed": "Scadenza cambiata",
    "types.new_comment": "Nuovo commento",
    "types.mentioned": "Menzionato",
    "types.review_requested": "Revisione richiesta",
    "types.task_completed": "Task completata",
    "types.task_failed": "Task fallita",
    "types.agent_blocked": "Agente bloccato",
    "types.agent_completed": "Agente ha finito",
    "types.reaction_added": "Ha reagito",
    "types.quick_create_done": "Creata con un agente",
    "types.quick_create_failed": "Creazione con agente non riuscita",
    "types.quick_create_unconfirmed": "Creazione con agente non confermata",
    "labels.set_status_to": "Ha impostato lo stato a",
    "labels.set_priority_to": "Ha impostato la priorità a",
    "labels.assigned_to": "Assegnata a {{name}}",
    "labels.removed_assignee": "Ha rimosso l'assegnatario",
    "labels.set_start_date_to": "Ha impostato l'inizio al {{date}}",
    "labels.removed_start_date": "Ha rimosso la data di inizio",
    "labels.set_due_date_to": "Ha impostato la scadenza al {{date}}",
    "labels.removed_due_date": "Ha rimosso la scadenza",
    "labels.reacted_to_comment": "Ha reagito {{emoji}} al tuo commento",
    "labels.created_with_agent": "Creata con un agente: {{identifier}}",
    "labels.failed_with_detail": "Non riuscita: {{detail}}",
    "errors.mark_read_failed": "Impossibile segnare come letto",
    "errors.archive_failed": "Archiviazione non riuscita",
    "errors.mark_done_failed": "Impossibile segnare come fatto",
    "errors.mark_all_read_failed": "Impossibile segnare tutto come letto",
    "errors.archive_all_failed": "Impossibile archiviare tutto",
    "errors.archive_all_read_failed": "Impossibile archiviare gli elementi letti",
    "errors.archive_completed_failed": "Impossibile archiviare i completati",
    "errors.unarchive_failed": "Impossibile togliere dall'archivio",
    "errors.archived_load_failed": "Impossibile caricare le notifiche archiviate",
}

T["auth"] = {
    "signin.title": "Accedi a Multica",
    "signin.description": "Inserisci la tua email per ricevere un codice di accesso",
    "signin.continue": "Continua",
    "signin.sending": "Invio del codice...",
    "signin.google": "Continua con Google",
    "verify.title": "Controlla la posta",
    "verify.description": "Abbiamo mandato un codice di verifica a {{email}}",
    "verify.resend": "Rimanda il codice",
    "verify.resend_cooldown": "Rimanda tra {{seconds}} s",
    "cli.title": "Autorizza la CLI",
    "cli.description": "Permetti alla CLI di accedere a Multica come {{email}}",
    "cli.authorize": "Autorizza",
    "cli.authorizing": "Autorizzazione...",
    "cli.different_account": "Usa un altro account",
    "common.back": "Indietro",
    "common.email": "Email",
    "common.email_placeholder": "tu@esempio.com",
    "common.email_required": "L'email è obbligatoria",
    "errors.server_unreachable": "Assicurati che il server sia in esecuzione.",
    "errors.send_failed": "Invio del codice non riuscito.",
    "errors.resend_failed": "Non è stato possibile rimandare il codice",
    "errors.code_invalid": "Codice non valido o scaduto",
    "errors.cli_auth_failed": "Autorizzazione della CLI non riuscita. Accedi di nuovo.",
    "web.prefer_desktop": "Preferisci l'app desktop?",
    "web.download": "Scarica",
    "web.desktop_handoff.preparing": "Preparazione dell'accesso da desktop...",
    "web.desktop_handoff.opening_title": "Apertura di Multica",
    "web.desktop_handoff.opening_description": "Dovresti vedere una richiesta di aprire l'app desktop di Multica. Se non accade nulla, usa il pulsante qui sotto.",
    "web.desktop_handoff.open_button": "Apri Multica Desktop",
    "web.desktop_handoff.failed_title": "Accesso non riuscito",
    "web.desktop_handoff.prepare_failed": "Preparazione dell'accesso da desktop non riuscita",
}

T["invite"] = {
    "header.back": "Indietro",
    "header.log_out": "Esci",
    "not_found.title": "Invito non trovato",
    "not_found.description": "Questo invito potrebbe essere scaduto, revocato, o non appartenere al tuo account.",
    "not_found.go_to_dashboard": "Vai alla dashboard",
    "accepted.title": "Sei entrato in {{workspace_name}}!",
    "accepted.redirecting": "Reindirizzamento al workspace...",
    "declined.title": "Invito rifiutato",
    "declined.description": "Non verrai aggiunto a questo workspace.",
    "declined.go_to_dashboard": "Vai alla dashboard",
    "main.join_title": "Entra in {{workspace_name}}",
    "main.fallback_workspace_name": "workspace",
    "main.invited_role_admin": "ti ha invitato a entrare come amministratore.",
    "main.invited_role_member": "ti ha invitato a entrare come membro.",
    "main.already_handled_accepted": "Questo invito è già stato accettato.",
    "main.already_handled_declined": "Questo invito è già stato rifiutato.",
    "main.expired": "Questo invito è scaduto.",
    "main.decline": "Rifiuta",
    "main.declining": "Rifiuto...",
    "main.accept": "Accetta ed entra",
    "main.joining": "Ingresso...",
    "errors.accept_failed": "Non è stato possibile accettare l'invito",
    "errors.decline_failed": "Non è stato possibile rifiutare l'invito",
    "batch.log_out": "Esci",
    "batch.empty_title": "Nessun invito in attesa",
    "batch.empty_hint": "Continua per creare il tuo workspace.",
    "batch.empty_continue": "Continua con la configurazione",
    "batch.title": "Sei stato invitato",
    "batch.subtitle": "Scegli i workspace in cui vuoi entrare. Gli altri li gestisci quando vuoi dalla barra laterale.",
    "batch.submit_skip": "Salta e crea il mio workspace",
    "batch.submit_join_one": "Entra in 1 workspace",
    "batch.submit_join_other": "Entra in {{count}} workspace",
    "batch.joining": "Ingresso...",
    "batch.error_generic": "Non è stato possibile elaborare gli inviti. Riprova.",
    "batch.row_workspace_fallback": "Workspace",
    "batch.row_inviter_fallback": "Qualcuno",
    "batch.row_invited_admin": "{{inviter}} ti ha invitato come amministratore",
    "batch.row_invited_member": "{{inviter}} ti ha invitato come membro",
}

T["search"] = {
    "title": "Cerca",
    "description": "Cerca fra pagine, issue, progetti e membri",
    "placeholder": "Scrivi un comando o cerca...",
    "groups.pages": "Pagine",
    "groups.commands": "Comandi",
    "groups.members": "Membri",
    "groups.projects": "Progetti",
    "groups.issues": "Issue",
    "groups.recent": "Recenti",
    "pages.inbox": "Inbox",
    "pages.my_issues": "Le mie issue",
    "pages.issues": "Issue",
    "pages.projects": "Progetti",
    "pages.agents": "Agenti",
    "pages.runtimes": "Runtime",
    "pages.skills": "Skill",
    "pages.settings": "Impostazioni",
    "commands.current_theme_aria": "Tema attuale",
    "commands.new_issue": "Nuova issue",
    "commands.new_project": "Nuovo progetto",
    "commands.copy_issue_link": "Copia il link della issue",
    "commands.copy_identifier": "Copia l'identificatore ({{identifier}})",
    "commands.fold_all_comments": "Chiudi tutti i commenti",
    "commands.unfold_all_comments": "Apri tutti i commenti",
    "commands.switch_to_light": "Passa al tema chiaro",
    "commands.switch_to_dark": "Passa al tema scuro",
    "commands.use_system_theme": "Usa il tema di sistema",
    "toast.link_copied": "Link copiato",
    "toast.copied_identifier": "Copiato {{identifier}}",
    "empty.no_results": "Nessun risultato.",
    "empty.type_to_search": "Scrivi per cercare fra issue e progetti",
    "trigger.label": "Cerca...",
}

T["my-issues"] = {
    "page.breadcrumb": "Le mie issue",
    "page.workspace_fallback": "Workspace",
    "page.empty_title": "Nessuna issue assegnata a te",
    "page.empty_description": "Le issue che crei o che ti vengono assegnate compaiono qui.",
    "header.scope.all_label": "Tutte",
    "header.scope.all_description": "Assegnate a me, create da me, o che coinvolgono i miei agenti e le mie squadre",
    "header.scope.assigned_label": "Assegnate",
    "header.scope.assigned_description": "Issue assegnate a me",
    "header.scope.created_label": "Create",
    "header.scope.created_description": "Issue che ho creato",
    "header.scope.agents_label": "I miei agenti e squadre",
    "header.scope.agents_description": "Issue assegnate ai miei agenti",
    "header.filter_button": "Filtra",
    "header.filter_status": "Stato",
    "header.filter_priority": "Priorità",
    "header.issue_count_one": "{{count}} issue",
    "header.issue_count_other": "{{count}} issue",
    "header.reset_filters": "Azzera tutti i filtri",
    "header.display_settings": "Impostazioni di visualizzazione",
    "header.grouping": "Raggruppamento",
    "header.group_status": "Stato",
    "header.group_assignee": "Assegnatario",
    "header.ordering": "Ordinamento",
    "header.ascending": "Crescente",
    "header.descending": "Decrescente",
    "header.card_properties": "Proprietà della card",
    "header.view_board": "Vista bacheca",
    "header.view_list": "Vista elenco",
    "header.view_swimlane": "Vista a corsie",
    "header.view_label": "Vista",
    "header.view_board_short": "Bacheca",
    "header.view_list_short": "Elenco",
    "header.view_swimlane_short": "Corsie",
    "header.sort_manual": "Manuale",
}

PLACEHOLDER = re.compile(r"\{\{[^}]+\}\}")


def apply(en, table, prefix=""):
    """Ricostruisce il ramo italiano seguendo la forma di `en`."""
    out = {}
    for k, v in en.items():
        path = f"{prefix}{k}"
        if isinstance(v, dict):
            out[k] = apply(v, table, path + ".")
        else:
            it = table.get(path, v)
            if isinstance(v, str):
                want, got = sorted(PLACEHOLDER.findall(v)), sorted(PLACEHOLDER.findall(it))
                if want != got:
                    raise SystemExit(f"segnaposto diversi su {path}: {want} vs {got}")
            out[k] = it
    return out


def count(o):
    return sum(count(v) if isinstance(v, dict) else 1 for v in o.values())


total_done = total_all = 0
for ns, table in sorted(T.items()):
    if ONLY and ns not in ONLY:
        continue
    en_path, it_path = LOC / "en" / f"{ns}.json", LOC / "it" / f"{ns}.json"
    en = json.loads(en_path.read_text())
    it = apply(en, table)
    it_path.write_text(json.dumps(it, ensure_ascii=False, indent=2) + "\n")
    n, tot = len(table), count(en)
    total_done += min(n, tot)
    total_all += tot
    print(f"  {ns:14} {min(n, tot)}/{tot} tradotte")

print(f"\nbatch: {total_done}/{total_all} stringhe")
