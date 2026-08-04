import { App } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui";
import type { HostDTO } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui/models";
import { confirmDialog } from "../confirmDialog";

function splitCSV(raw: string): string[] {
    return raw
        .split(",")
        .map((s) => s.trim())
        .filter((s) => s.length > 0);
}

export function renderHostsPage(root: HTMLElement) {
    root.innerHTML = `
        <h1>Hosts</h1>
        <div id="error" class="error" hidden></div>
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
        <form id="add-host-form" class="entity-form">
            <h2 id="form-title">Novo host</h2>
            <label>Patterns (separados por vírgula) <input id="patterns" required/></label>
            <label>HostName <input id="hostname"/></label>
            <label>User <input id="user"/></label>
            <label>Port <input id="port"/></label>
            <div class="form-buttons">
                <button type="submit" id="submit-btn">Adicionar</button>
                <button type="button" id="cancel-edit-btn" hidden>Cancelar edição</button>
            </div>
        </form>
    `;

    const tableBody = root.querySelector("#hosts-body") as HTMLTableSectionElement;
    const errorBox = root.querySelector("#error") as HTMLDivElement;
    const form = root.querySelector("#add-host-form") as HTMLFormElement;
    const formTitle = root.querySelector("#form-title") as HTMLHeadingElement;
    const patternsInput = root.querySelector("#patterns") as HTMLInputElement;
    const hostNameInput = root.querySelector("#hostname") as HTMLInputElement;
    const userInput = root.querySelector("#user") as HTMLInputElement;
    const portInput = root.querySelector("#port") as HTMLInputElement;
    const submitBtn = root.querySelector("#submit-btn") as HTMLButtonElement;
    const cancelEditBtn = root.querySelector("#cancel-edit-btn") as HTMLButtonElement;

    // Quando não-nulo, o formulário está em modo de edição de um host
    // existente (ReplaceHost) em vez de criação (AddHost).
    let editing: HostDTO | null = null;

    function showError(err: unknown) {
        errorBox.textContent = err instanceof Error ? err.message : String(err);
        errorBox.hidden = false;
    }

    function clearError() {
        errorBox.hidden = true;
        errorBox.textContent = "";
    }

    function startEdit(host: HostDTO) {
        editing = host;
        formTitle.textContent = "Editar host " + host.patterns.join(", ");
        patternsInput.value = host.patterns.join(", ");
        hostNameInput.value = host.hostName ?? "";
        userInput.value = host.user ?? "";
        portInput.value = host.port ?? "";
        submitBtn.textContent = "Salvar";
        cancelEditBtn.hidden = false;
    }

    function stopEdit() {
        editing = null;
        formTitle.textContent = "Novo host";
        form.reset();
        submitBtn.textContent = "Adicionar";
        cancelEditBtn.hidden = true;
    }

    function renderHosts(hosts: HostDTO[]) {
        tableBody.innerHTML = "";
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
            editBtn.addEventListener("click", () => startEdit(h));

            const removeBtn = document.createElement("button");
            removeBtn.type = "button";
            removeBtn.textContent = "Remover";
            removeBtn.addEventListener("click", () => removeHost(h));

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
            await reload();
        } catch (err) {
            showError(err);
        }
    }

    async function reload() {
        try {
            const hosts = await App.ListHosts();
            renderHosts(hosts);
            clearError();
        } catch (err) {
            showError(err);
        }
    }

    cancelEditBtn.addEventListener("click", () => stopEdit());

    form.addEventListener("submit", async (ev) => {
        ev.preventDefault();
        const patterns = splitCSV(patternsInput.value);
        if (patterns.length === 0) {
            showError(new Error("informe ao menos um pattern"));
            return;
        }
        const spec = {
            patterns,
            hostName: hostNameInput.value,
            user: userInput.value,
            port: portInput.value,
        };

        try {
            if (editing) {
                const ok = await confirmDialog({
                    title: "Substituir host existente?",
                    body: `As diretivas atuais de ${editing.patterns.join(", ")} em ${editing.sourceFile} serão descartadas e reescritas.`,
                    danger: true,
                });
                if (!ok) return;
                await App.ReplaceHost({
                    sourceFile: editing.sourceFile,
                    oldPatterns: editing.patterns,
                    newSpec: spec,
                });
            } else {
                await App.AddHost(spec);
            }
            stopEdit();
            await reload();
        } catch (err) {
            showError(err);
        }
    });

    reload();
}
