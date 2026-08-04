import { App } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui";
import type {
    HostDTO,
    HostLinkDTO,
    KeyDTO,
} from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui/models";
import { confirmDialog } from "../confirmDialog";
import { createFeedback } from "../feedback";
import { formModal } from "../modal";

function splitCSV(raw: string): string[] {
    return raw
        .split(",")
        .map((s) => s.trim())
        .filter((s) => s.length > 0);
}

export function renderHostsPage(root: HTMLElement) {
    root.innerHTML = `
        <div class="page-header">
            <h1>Hosts</h1>
            <button type="button" id="new-host-btn" class="btn-primary">+ Novo host</button>
        </div>
        <div id="error" class="error" hidden></div>
        <div id="success" class="saved" hidden></div>
        <div class="table-scroll">
            <table class="data-table">
                <thead>
                <tr>
                    <th>Patterns</th>
                    <th>HostName</th>
                    <th>User</th>
                    <th>Port</th>
                    <th>IdentityFile</th>
                    <th>Arquivo</th>
                    <th>Ações</th>
                </tr>
                </thead>
                <tbody id="hosts-body"></tbody>
            </table>
        </div>
        <p id="empty-state" class="muted" hidden>Nenhum host configurado ainda.</p>

        <div class="page-header">
            <h2>Vínculos com chaves de agente</h2>
        </div>
        <p class="muted">
            Um host vinculado a uma chave de agente não precisa de <code>IdentityFile</code> local —
            o OpenSSH oferece a identidade automaticamente através do agente.
        </p>
        <div class="table-scroll">
            <table class="data-table">
                <thead>
                <tr>
                    <th>Host</th>
                    <th>Chave de agente</th>
                    <th>Notas</th>
                    <th>Ações</th>
                </tr>
                </thead>
                <tbody id="links-body"></tbody>
            </table>
        </div>
        <p id="links-empty-state" class="muted" hidden>Nenhum vínculo host/chave de agente ainda.</p>
    `;

    const tableBody = root.querySelector("#hosts-body") as HTMLTableSectionElement;
    const emptyState = root.querySelector("#empty-state") as HTMLParagraphElement;
    const newHostBtn = root.querySelector("#new-host-btn") as HTMLButtonElement;
    const linksBody = root.querySelector("#links-body") as HTMLTableSectionElement;
    const linksEmptyState = root.querySelector("#links-empty-state") as HTMLParagraphElement;
    const { showError, showSuccess, clear } = createFeedback(root);

    // hostKeyLabels mapeia App.HostKey(patterns) -> "patterns, ..." pra
    // exibir o host legível na tabela de vínculos, sem repetir a chamada
    // Go->JS por linha renderizada.
    let hostKeyLabels = new Map<string, string>();

    // openLinkModal oferece um formulário mínimo (select de chaves de
    // agente disponíveis + notas) pra vincular um host — sem exigir
    // IdentityFile, mantendo o vínculo puramente informativo (spec
    // ssh-agent-support, requisito 5).
    async function openLinkModal(host: HostDTO) {
        let agentKeys: KeyDTO[];
        try {
            agentKeys = (await App.ListKeys()).filter((k) => k.source === "agent");
        } catch (err) {
            showError(err);
            return;
        }
        if (agentKeys.length === 0) {
            showError(new Error("Nenhuma chave de agente disponível pra vincular."));
            return;
        }

        const ok = await formModal({
            title: "Vincular chave de agente a " + host.patterns.join(", "),
            submitLabel: "Vincular",
            fields: [
                {
                    name: "fingerprint",
                    label: "Chave de agente",
                    type: "select",
                    required: true,
                    options: agentKeys.map((k) => ({
                        value: k.metadata.fingerprint,
                        label: (k.metadata.label || k.metadata.fingerprint) + (k.agentName ? ` (${k.agentName})` : ""),
                    })),
                },
                { name: "notes", label: "Notas" },
            ],
            onSubmit: async (values) => {
                const hostKey = await App.HostKey(host.patterns);
                await App.LinkHostKey({ hostKey, agentKeyFingerprint: values.fingerprint, notes: values.notes });
            },
        });
        if (ok) {
            showSuccess("Vínculo criado.");
            await reloadLinks();
        }
    }

    async function unlinkHostKey(link: HostLinkDTO) {
        const ok = await confirmDialog({
            title: "Remover vínculo?",
            body: `O vínculo entre o host e a chave de agente ${link.agentKeyFingerprint} será removido. Isso não altera ~/.ssh/config.`,
        });
        if (!ok) return;
        try {
            await App.UnlinkHostKey({ hostKey: link.hostKey, agentKeyFingerprint: link.agentKeyFingerprint });
            showSuccess("Vínculo removido.");
            await reloadLinks();
        } catch (err) {
            showError(err);
        }
    }

    function hostLabelForKey(hostKey: string): string {
        return hostKeyLabels.get(hostKey) ?? (hostKey || "(host)");
    }

    async function refreshHostKeyLabels(hosts: HostDTO[]): Promise<void> {
        const entries: [string, string][] = [];
        for (const h of hosts) entries.push([await App.HostKey(h.patterns), h.patterns.join(", ")]);
        hostKeyLabels = new Map(entries);
    }

    function renderLinks(links: HostLinkDTO[]) {
        linksBody.innerHTML = "";
        linksEmptyState.hidden = links.length > 0;
        for (const link of links) {
            const row = document.createElement("tr");

            const hostCell = document.createElement("td");
            hostCell.textContent = hostLabelForKey(link.hostKey);
            if (link.orphan) {
                const orphanBadge = document.createElement("span");
                orphanBadge.className = "badge badge-warn";
                orphanBadge.textContent = "órfão — host não encontrado";
                orphanBadge.style.marginLeft = "0.5em";
                hostCell.appendChild(orphanBadge);
            }

            const keyCell = document.createElement("td");
            keyCell.textContent = link.agentKeyFingerprint;
            keyCell.className = "muted";

            const notesCell = document.createElement("td");
            notesCell.textContent = link.notes || "-";

            const actionsCell = document.createElement("td");
            const unlinkBtn = document.createElement("button");
            unlinkBtn.type = "button";
            unlinkBtn.textContent = "Desvincular";
            unlinkBtn.addEventListener("click", () => void unlinkHostKey(link));
            actionsCell.appendChild(unlinkBtn);

            row.append(hostCell, keyCell, notesCell, actionsCell);
            linksBody.appendChild(row);
        }
    }

    async function reloadLinks() {
        try {
            const links = await App.ListHostLinks();
            renderLinks(links);
        } catch (err) {
            showError(err);
        }
    }

    // openHostModal cobre criação (host === null) e edição (host !== null)
    // com o mesmo formulário — só o título, os valores default e o binding
    // chamado no submit mudam.
    async function openHostModal(host: HostDTO | null) {
        const ok = await formModal({
            title: host ? "Editar host " + host.patterns.join(", ") : "Novo host",
            submitLabel: host ? "Salvar" : "Adicionar",
            fields: [
                {
                    name: "patterns",
                    label: "Patterns (separados por vírgula)",
                    value: host?.patterns.join(", ") ?? "",
                    required: true,
                },
                { name: "hostname", label: "HostName", value: host?.hostName ?? "" },
                { name: "user", label: "User", value: host?.user ?? "" },
                { name: "port", label: "Port", value: host?.port ?? "" },
            ],
            onSubmit: async (values) => {
                const patterns = splitCSV(values.patterns);
                if (patterns.length === 0) {
                    throw new Error("informe ao menos um pattern");
                }
                const spec = {
                    patterns,
                    hostName: values.hostname,
                    user: values.user,
                    port: values.port,
                    // O formulário não expõe IdentityFile como campo editável
                    // (só a CLI/TUI mexem nisso hoje) — preservar o valor
                    // existente evita que editar um host descarte suas
                    // diretivas IdentityFile.
                    identityFile: host?.identityFile ?? undefined,
                };
                if (host) {
                    const confirmed = await confirmDialog({
                        title: "Substituir host existente?",
                        body: `As diretivas atuais de ${host.patterns.join(", ")} em ${host.sourceFile} serão descartadas e reescritas.`,
                        danger: true,
                    });
                    if (!confirmed) return; // fecha o confirm, mantém o formulário aberto
                    await App.ReplaceHost({ sourceFile: host.sourceFile, oldPatterns: host.patterns, newSpec: spec });
                } else {
                    await App.AddHost(spec);
                }
            },
        });
        if (ok) {
            showSuccess(host ? "Host atualizado." : "Host adicionado.");
            await reload();
        }
    }

    function renderHosts(hosts: HostDTO[]) {
        tableBody.innerHTML = "";
        emptyState.hidden = hosts.length > 0;
        for (const h of hosts) {
            const row = document.createElement("tr");
            const cells = [
                h.patterns.join(", "),
                h.hostName ?? "-",
                h.user ?? "-",
                h.port ?? "-",
                (h.identityFile ?? []).join(", ") || "-",
                h.sourceFile,
            ];
            for (const text of cells) {
                const cell = document.createElement("td");
                cell.textContent = text;
                row.appendChild(cell);
            }

            const actionsCell = document.createElement("td");
            const editBtn = document.createElement("button");
            editBtn.type = "button";
            editBtn.textContent = "Editar";
            editBtn.addEventListener("click", () => void openHostModal(h));

            const removeBtn = document.createElement("button");
            removeBtn.type = "button";
            removeBtn.textContent = "Remover";
            removeBtn.addEventListener("click", () => void removeHost(h));

            const linkBtn = document.createElement("button");
            linkBtn.type = "button";
            linkBtn.textContent = "Vincular chave de agente";
            linkBtn.addEventListener("click", () => void openLinkModal(h));

            actionsCell.append(editBtn, removeBtn, linkBtn);
            row.appendChild(actionsCell);
            tableBody.appendChild(row);
        }
    }

    async function removeHost(host: HostDTO) {
        const ok = await confirmDialog({
            title: "Remover host?",
            body: `O bloco Host ${host.patterns.join(", ")} em ${host.sourceFile} será removido permanentemente.`,
            danger: true,
        });
        if (!ok) return;

        try {
            await App.RemoveHost({ sourceFile: host.sourceFile, patterns: host.patterns });
            showSuccess("Host removido.");
            await reload();
        } catch (err) {
            showError(err);
        }
    }

    async function reload() {
        try {
            const hosts = await App.ListHosts();
            renderHosts(hosts);
            await refreshHostKeyLabels(hosts);
            await reloadLinks();
            clear();
        } catch (err) {
            showError(err);
        }
    }

    newHostBtn.addEventListener("click", () => void openHostModal(null));

    reload();
}
