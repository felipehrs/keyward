import { App } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui";
import type { KeyDTO } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui/models";
import { confirmDialog } from "../confirmDialog";
import { createFeedback } from "../feedback";
import { formModal, ModalCancelled } from "../modal";

function formatDate(iso: string | undefined): string {
    if (!iso) return "-";
    return iso.slice(0, 10);
}

function statusLabel(k: KeyDTO): { text: string; className: string } {
    if (k.isExpired) return { text: "EXPIRADA", className: "badge badge-danger" };
    if (k.isExpiringSoon) return { text: "EXPIRANDO", className: "badge badge-warn" };
    switch (k.status) {
        case "unregistered":
            return { text: "sem registro de metadata", className: "badge badge-muted" };
        case "missingFile":
            return { text: "arquivo ausente em disco", className: "badge badge-muted" };
        default:
            return { text: "OK", className: "badge badge-ok" };
    }
}

export function renderKeysPage(root: HTMLElement) {
    root.innerHTML = `
        <div class="page-header">
            <h1>Chaves</h1>
            <button type="button" id="generate-btn" class="btn-primary">+ Gerar chave</button>
        </div>
        <div id="error" class="error" hidden></div>
        <div id="success" class="saved" hidden></div>
        <div class="table-scroll">
            <table class="data-table">
                <thead>
                <tr>
                    <th>Rótulo</th>
                    <th>Fingerprint</th>
                    <th>Algoritmo</th>
                    <th>Status</th>
                    <th>Expira em</th>
                    <th>Ações</th>
                </tr>
                </thead>
                <tbody id="keys-body"></tbody>
            </table>
        </div>
        <p id="empty-state" class="muted" hidden>Nenhuma chave encontrada.</p>
    `;

    const tableBody = root.querySelector("#keys-body") as HTMLTableSectionElement;
    const emptyState = root.querySelector("#empty-state") as HTMLParagraphElement;
    const generateBtn = root.querySelector("#generate-btn") as HTMLButtonElement;
    const { showError, showSuccess, clear } = createFeedback(root);

    // submitGenerate é recursivo: se o core recusar por já existir um
    // arquivo com esse nome, abre um confirm por cima do próprio modal de
    // geração e, se aceito, tenta de novo com overwrite=true. Recusar o
    // confirm lança ModalCancelled — formModal entende isso como "continue
    // aberto, sem mostrar erro", não como sucesso nem como falha real.
    async function submitGenerate(values: Record<string, string>, overwrite: boolean): Promise<void> {
        const input = {
            algorithm: values.algorithm,
            rsaBits: values.rsaBits ? Number(values.rsaBits) : undefined,
            passphrase: values.passphrase,
            comment: "",
            fileName: values.fileName,
            overwrite,
            directory: values.directory,
            label: values.label,
            notes: values.notes,
            expiresAt: values.expiresAt || undefined,
        };
        try {
            await App.GenerateKey(input);
        } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            if (!overwrite && message.includes("já existe (use Overwrite")) {
                const confirmed = await confirmDialog({
                    title: "Sobrescrever chave existente?",
                    body: message,
                    danger: true,
                });
                if (confirmed) {
                    await submitGenerate(values, true);
                    return;
                }
                throw new ModalCancelled();
            }
            throw err;
        }
    }

    async function openGenerateModal() {
        const ok = await formModal({
            title: "Gerar chave",
            submitLabel: "Gerar",
            wide: true,
            fields: [
                {
                    name: "algorithm",
                    label: "Algoritmo",
                    type: "select",
                    value: "",
                    options: [
                        { value: "", label: "(default das configurações)" },
                        { value: "ed25519", label: "ed25519" },
                        { value: "rsa", label: "rsa" },
                    ],
                },
                { name: "rsaBits", label: "RSA bits (só se algoritmo=rsa)", type: "number", placeholder: "4096" },
                { name: "label", label: "Rótulo" },
                { name: "notes", label: "Notas" },
                { name: "fileName", label: "Nome do arquivo", placeholder: "id_ed25519" },
                { name: "directory", label: "Diretório", placeholder: "~/.ssh" },
                { name: "passphrase", label: "Passphrase", type: "password" },
                { name: "expiresAt", label: "Expira em", type: "date" },
            ],
            onSubmit: (values) => submitGenerate(values, false),
        });
        if (ok) {
            showSuccess("Chave gerada.");
            await reload();
        }
    }

    async function openRegisterModal(key: KeyDTO) {
        const ok = await formModal({
            title: "Registrar " + (key.privateKeyPath || key.publicKeyPath),
            submitLabel: "Registrar",
            fields: [
                { name: "label", label: "Rótulo" },
                { name: "notes", label: "Notas" },
                { name: "expiresAt", label: "Expira em", type: "date" },
            ],
            onSubmit: async (values) => {
                await App.RegisterKey({
                    privateKeyPath: key.privateKeyPath,
                    label: values.label,
                    notes: values.notes,
                    expiresAt: values.expiresAt || undefined,
                });
            },
        });
        if (ok) {
            showSuccess("Chave registrada.");
            await reload();
        }
    }

    async function openEditModal(key: KeyDTO) {
        const ok = await formModal({
            title: "Editar " + (key.metadata.label || key.metadata.fingerprint),
            submitLabel: "Salvar",
            fields: [
                { name: "label", label: "Rótulo", value: key.metadata.label ?? "" },
                // Bug corrigido: o formulário original zerava esse campo em
                // vez de carregar o valor atual — editar só o rótulo apagava
                // as notas da chave, porque o submit sempre manda "notes".
                { name: "notes", label: "Notas", value: key.metadata.notes ?? "" },
                {
                    name: "expiresAction",
                    label: "Expiração",
                    type: "select",
                    value: "keep",
                    options: [
                        { value: "keep", label: "Manter atual" },
                        { value: "clear", label: "Remover expiração" },
                        { value: "set", label: "Definir nova data" },
                    ],
                },
                {
                    name: "expiresValue",
                    label: "Nova data",
                    type: "date",
                    visibleWhen: (values) => values.expiresAction === "set",
                },
            ],
            onSubmit: async (values) => {
                await App.UpdateKeyMetadata(key.metadata.fingerprint, {
                    label: values.label,
                    notes: values.notes,
                    expiresAt: values.expiresAction as "keep" | "clear" | "set",
                    expiresAtValue: values.expiresValue || undefined,
                });
            },
        });
        if (ok) {
            showSuccess("Metadata atualizada.");
            await reload();
        }
    }

    async function unregisterKey(key: KeyDTO) {
        const ok = await confirmDialog({
            title: "Remover registro de metadata?",
            body: `O arquivo da chave permanece em disco — só o registro de metadata (rótulo, notas, expiração) é removido. A chave volta a aparecer como "sem registro de metadata".`,
        });
        if (!ok) return;
        try {
            await App.UnregisterKey(key.metadata.fingerprint);
            showSuccess("Registro de metadata removido.");
            await reload();
        } catch (err) {
            showError(err);
        }
    }

    function renderKeys(keys: KeyDTO[]) {
        tableBody.innerHTML = "";
        emptyState.hidden = keys.length > 0;
        for (const k of keys) {
            const row = document.createElement("tr");

            const label = document.createElement("td");
            label.textContent = k.metadata.label || "-";

            const fingerprint = document.createElement("td");
            fingerprint.textContent = k.metadata.fingerprint || "-";
            fingerprint.className = "muted";

            const algorithm = document.createElement("td");
            algorithm.textContent = k.algorithm || "-";

            const status = document.createElement("td");
            const { text, className } = statusLabel(k);
            const badge = document.createElement("span");
            badge.className = className;
            badge.textContent = text;
            status.appendChild(badge);

            const expires = document.createElement("td");
            expires.textContent = formatDate(k.metadata.expiresAt);

            const actions = document.createElement("td");
            if (k.status === "unregistered") {
                const registerBtn = document.createElement("button");
                registerBtn.type = "button";
                registerBtn.textContent = "Registrar";
                registerBtn.addEventListener("click", () => void openRegisterModal(k));
                actions.appendChild(registerBtn);
            } else if (k.status === "ok") {
                const editBtn = document.createElement("button");
                editBtn.type = "button";
                editBtn.textContent = "Editar";
                editBtn.addEventListener("click", () => void openEditModal(k));

                const unregisterBtn = document.createElement("button");
                unregisterBtn.type = "button";
                unregisterBtn.textContent = "Remover registro";
                unregisterBtn.addEventListener("click", () => void unregisterKey(k));

                actions.append(editBtn, unregisterBtn);
            }

            row.append(label, fingerprint, algorithm, status, expires, actions);
            tableBody.appendChild(row);
        }
    }

    async function reload() {
        try {
            const keys = await App.ListKeys();
            renderKeys(keys);
            clear();
        } catch (err) {
            showError(err);
        }
    }

    generateBtn.addEventListener("click", () => void openGenerateModal());

    reload();
}
