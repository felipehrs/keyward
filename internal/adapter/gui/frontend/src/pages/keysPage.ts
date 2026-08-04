import { App } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui";
import type { KeyDTO } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui/models";
import { confirmDialog } from "../confirmDialog";

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
        <h1>Chaves</h1>
        <div id="error" class="error" hidden></div>
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

        <form id="generate-form" class="entity-form">
            <h2>Gerar chave</h2>
            <label>Algoritmo
                <select id="algorithm">
                    <option value="">(default das configurações)</option>
                    <option value="ed25519">ed25519</option>
                    <option value="rsa">rsa</option>
                </select>
            </label>
            <label>RSA bits (só se algoritmo=rsa) <input id="rsaBits" type="number" placeholder="4096"/></label>
            <label>Rótulo <input id="label"/></label>
            <label>Notas <input id="notes"/></label>
            <label>Nome do arquivo <input id="fileName" placeholder="id_ed25519"/></label>
            <label>Diretório <input id="directory" placeholder="~/.ssh"/></label>
            <label>Passphrase <input id="passphrase" type="password"/></label>
            <label>Expira em <input id="expiresAt" type="date"/></label>
            <div class="form-buttons">
                <button type="submit">Gerar</button>
            </div>
        </form>

        <form id="register-form" class="entity-form" hidden>
            <h2 id="register-title">Registrar chave órfã</h2>
            <label>Rótulo <input id="register-label"/></label>
            <label>Notas <input id="register-notes"/></label>
            <label>Expira em <input id="register-expiresAt" type="date"/></label>
            <div class="form-buttons">
                <button type="submit">Registrar</button>
                <button type="button" id="cancel-register-btn">Cancelar</button>
            </div>
        </form>

        <form id="edit-form" class="entity-form" hidden>
            <h2 id="edit-title">Editar metadata</h2>
            <label>Rótulo <input id="edit-label"/></label>
            <label>Notas <input id="edit-notes"/></label>
            <label>Expiração
                <select id="edit-expires-action">
                    <option value="keep">Manter atual</option>
                    <option value="clear">Remover expiração</option>
                    <option value="set">Definir nova data</option>
                </select>
            </label>
            <label id="edit-expires-value-wrap" hidden>Nova data <input id="edit-expires-value" type="date"/></label>
            <div class="form-buttons">
                <button type="submit">Salvar</button>
                <button type="button" id="cancel-edit-btn">Cancelar</button>
            </div>
        </form>
    `;

    const tableBody = root.querySelector("#keys-body") as HTMLTableSectionElement;
    const errorBox = root.querySelector("#error") as HTMLDivElement;

    const generateForm = root.querySelector("#generate-form") as HTMLFormElement;
    const algorithmInput = root.querySelector("#algorithm") as HTMLSelectElement;
    const rsaBitsInput = root.querySelector("#rsaBits") as HTMLInputElement;
    const labelInput = root.querySelector("#label") as HTMLInputElement;
    const notesInput = root.querySelector("#notes") as HTMLInputElement;
    const fileNameInput = root.querySelector("#fileName") as HTMLInputElement;
    const directoryInput = root.querySelector("#directory") as HTMLInputElement;
    const passphraseInput = root.querySelector("#passphrase") as HTMLInputElement;
    const expiresAtInput = root.querySelector("#expiresAt") as HTMLInputElement;

    const registerForm = root.querySelector("#register-form") as HTMLFormElement;
    const registerTitle = root.querySelector("#register-title") as HTMLHeadingElement;
    const registerLabelInput = root.querySelector("#register-label") as HTMLInputElement;
    const registerNotesInput = root.querySelector("#register-notes") as HTMLInputElement;
    const registerExpiresAtInput = root.querySelector("#register-expiresAt") as HTMLInputElement;
    const cancelRegisterBtn = root.querySelector("#cancel-register-btn") as HTMLButtonElement;
    let registerTarget: KeyDTO | null = null;

    const editForm = root.querySelector("#edit-form") as HTMLFormElement;
    const editTitle = root.querySelector("#edit-title") as HTMLHeadingElement;
    const editLabelInput = root.querySelector("#edit-label") as HTMLInputElement;
    const editNotesInput = root.querySelector("#edit-notes") as HTMLInputElement;
    const editExpiresAction = root.querySelector("#edit-expires-action") as HTMLSelectElement;
    const editExpiresValueWrap = root.querySelector("#edit-expires-value-wrap") as HTMLLabelElement;
    const editExpiresValue = root.querySelector("#edit-expires-value") as HTMLInputElement;
    const cancelEditBtn = root.querySelector("#cancel-edit-btn") as HTMLButtonElement;
    let editTarget: KeyDTO | null = null;

    function showError(err: unknown) {
        errorBox.textContent = err instanceof Error ? err.message : String(err);
        errorBox.hidden = false;
    }

    function clearError() {
        errorBox.hidden = true;
        errorBox.textContent = "";
    }

    function startRegister(key: KeyDTO) {
        registerTarget = key;
        registerTitle.textContent = "Registrar " + (key.privateKeyPath || key.publicKeyPath);
        registerForm.hidden = false;
        generateForm.hidden = true;
    }

    function stopRegister() {
        registerTarget = null;
        registerForm.reset();
        registerForm.hidden = true;
        generateForm.hidden = false;
    }

    function startEdit(key: KeyDTO) {
        editTarget = key;
        editTitle.textContent = "Editar " + (key.metadata.label || key.metadata.fingerprint);
        editLabelInput.value = key.metadata.label ?? "";
        editNotesInput.value = "";
        editExpiresAction.value = "keep";
        editExpiresValueWrap.hidden = true;
        editExpiresValue.value = "";
        editForm.hidden = false;
        generateForm.hidden = true;
    }

    function stopEdit() {
        editTarget = null;
        editForm.reset();
        editExpiresValueWrap.hidden = true;
        editForm.hidden = true;
        generateForm.hidden = false;
    }

    async function unregisterKey(key: KeyDTO) {
        const ok = await confirmDialog({
            title: "Remover registro de metadata?",
            body: `O arquivo da chave permanece em disco — só o registro de metadata (rótulo, notas, expiração) é removido. A chave volta a aparecer como "sem registro de metadata".`,
        });
        if (!ok) return;
        try {
            await App.UnregisterKey(key.metadata.fingerprint);
            await reload();
        } catch (err) {
            showError(err);
        }
    }

    function renderKeys(keys: KeyDTO[]) {
        tableBody.innerHTML = "";
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
                registerBtn.addEventListener("click", () => startRegister(k));
                actions.appendChild(registerBtn);
            } else if (k.status === "ok") {
                const editBtn = document.createElement("button");
                editBtn.type = "button";
                editBtn.textContent = "Editar";
                editBtn.addEventListener("click", () => startEdit(k));

                const unregisterBtn = document.createElement("button");
                unregisterBtn.type = "button";
                unregisterBtn.textContent = "Remover registro";
                unregisterBtn.addEventListener("click", () => unregisterKey(k));

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
            clearError();
        } catch (err) {
            showError(err);
        }
    }

    cancelRegisterBtn.addEventListener("click", () => stopRegister());

    registerForm.addEventListener("submit", async (ev) => {
        ev.preventDefault();
        if (!registerTarget) return;
        try {
            await App.RegisterKey({
                privateKeyPath: registerTarget.privateKeyPath,
                label: registerLabelInput.value,
                notes: registerNotesInput.value,
                expiresAt: registerExpiresAtInput.value || undefined,
            });
            stopRegister();
            await reload();
        } catch (err) {
            showError(err);
        }
    });

    async function submitGenerate(overwrite: boolean) {
        const input = {
            algorithm: algorithmInput.value,
            rsaBits: rsaBitsInput.value ? Number(rsaBitsInput.value) : undefined,
            passphrase: passphraseInput.value,
            comment: "",
            fileName: fileNameInput.value,
            overwrite,
            directory: directoryInput.value,
            label: labelInput.value,
            notes: notesInput.value,
            expiresAt: expiresAtInput.value || undefined,
        };

        try {
            await App.GenerateKey(input);
            generateForm.reset();
            clearError();
            await reload();
        } catch (err) {
            const message = err instanceof Error ? err.message : String(err);
            if (!overwrite && message.includes("já existe (use Overwrite")) {
                const ok = await confirmDialog({
                    title: "Sobrescrever chave existente?",
                    body: message,
                    danger: true,
                });
                if (ok) {
                    await submitGenerate(true);
                    return;
                }
                return;
            }
            showError(err);
        }
    }

    generateForm.addEventListener("submit", (ev) => {
        ev.preventDefault();
        void submitGenerate(false);
    });

    editExpiresAction.addEventListener("change", () => {
        editExpiresValueWrap.hidden = editExpiresAction.value !== "set";
    });

    cancelEditBtn.addEventListener("click", () => stopEdit());

    editForm.addEventListener("submit", async (ev) => {
        ev.preventDefault();
        if (!editTarget) return;
        try {
            await App.UpdateKeyMetadata(editTarget.metadata.fingerprint, {
                label: editLabelInput.value,
                notes: editNotesInput.value,
                expiresAt: editExpiresAction.value as "keep" | "clear" | "set",
                expiresAtValue: editExpiresValue.value || undefined,
            });
            stopEdit();
            await reload();
        } catch (err) {
            showError(err);
        }
    });

    reload();
}
