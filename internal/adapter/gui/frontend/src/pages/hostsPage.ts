import { App } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui";
import type { HostDTO } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui/models";
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
    `;

    const tableBody = root.querySelector("#hosts-body") as HTMLTableSectionElement;
    const emptyState = root.querySelector("#empty-state") as HTMLParagraphElement;
    const newHostBtn = root.querySelector("#new-host-btn") as HTMLButtonElement;
    const { showError, showSuccess, clear } = createFeedback(root);

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

            actionsCell.append(editBtn, removeBtn);
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
            clear();
        } catch (err) {
            showError(err);
        }
    }

    newHostBtn.addEventListener("click", () => void openHostModal(null));

    reload();
}
