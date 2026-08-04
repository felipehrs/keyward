import { App } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui";
import type {
    HostDTO,
    KeyDTO,
    ImportPreviewDTO,
    ImportResultDTO,
} from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui/models";
import { confirmDialog } from "../confirmDialog";

export function renderBackupPage(root: HTMLElement) {
    root.innerHTML = `
        <h1>Backup</h1>
        <div class="tabs">
            <button type="button" id="tab-export" class="tab-btn active">Exportar</button>
            <button type="button" id="tab-import" class="tab-btn">Importar</button>
        </div>
        <div id="tab-content"></div>
    `;

    const tabExportBtn = root.querySelector("#tab-export") as HTMLButtonElement;
    const tabImportBtn = root.querySelector("#tab-import") as HTMLButtonElement;
    const tabContent = root.querySelector("#tab-content") as HTMLElement;

    function showExport() {
        tabExportBtn.classList.add("active");
        tabImportBtn.classList.remove("active");
        renderExportTab(tabContent);
    }

    function showImport() {
        tabImportBtn.classList.add("active");
        tabExportBtn.classList.remove("active");
        renderImportTab(tabContent);
    }

    tabExportBtn.addEventListener("click", showExport);
    tabImportBtn.addEventListener("click", showImport);

    showExport();
}

function renderExportTab(root: HTMLElement) {
    root.innerHTML = `
        <div id="error" class="error" hidden></div>
        <div id="success" class="saved" hidden></div>
        <div id="private-warning" class="warning-high-contrast" hidden>
            ATENÇÃO: o pacote incluirá chave(s) privada(s) em texto claro — armazene o arquivo
            resultante com segurança.
        </div>

        <h2>Hosts</h2>
        <div id="hosts-list" class="checkbox-list"></div>

        <h2>Chaves</h2>
        <div id="keys-list" class="checkbox-list"></div>

        <label class="checkbox-row">
            <input type="checkbox" id="include-settings"/> Incluir configurações (AppSettings)
        </label>

        <label class="entity-form-row">Destino
            <div class="dest-row">
                <input id="dest-path" placeholder="ex: C:\\backup\\keyward-backup.tar.gz"/>
                <button type="button" id="choose-dest-btn">Escolher arquivo…</button>
            </div>
        </label>

        <div class="form-buttons">
            <button type="button" id="export-btn">Exportar</button>
        </div>
    `;

    const errorBox = root.querySelector("#error") as HTMLDivElement;
    const successBox = root.querySelector("#success") as HTMLDivElement;
    const privateWarning = root.querySelector("#private-warning") as HTMLDivElement;
    const hostsList = root.querySelector("#hosts-list") as HTMLDivElement;
    const keysList = root.querySelector("#keys-list") as HTMLDivElement;
    const includeSettings = root.querySelector("#include-settings") as HTMLInputElement;
    const destPath = root.querySelector("#dest-path") as HTMLInputElement;
    const chooseDestBtn = root.querySelector("#choose-dest-btn") as HTMLButtonElement;
    const exportBtn = root.querySelector("#export-btn") as HTMLButtonElement;

    let hosts: HostDTO[] = [];
    let keys: KeyDTO[] = [];
    const selectedHosts = new Set<number>();
    const selectedKeys = new Map<number, boolean>(); // index -> includePrivate

    function showError(err: unknown) {
        errorBox.textContent = err instanceof Error ? err.message : String(err);
        errorBox.hidden = false;
        successBox.hidden = true;
    }

    function clearError() {
        errorBox.hidden = true;
        errorBox.textContent = "";
    }

    function showSuccess(message: string) {
        successBox.textContent = message;
        successBox.hidden = false;
    }

    function updatePrivateWarning() {
        const anyPrivate = [...selectedKeys.values()].some((v) => v);
        privateWarning.hidden = !anyPrivate;
    }

    function renderHostsList() {
        hostsList.innerHTML = "";
        hosts.forEach((h, i) => {
            const row = document.createElement("label");
            row.className = "checkbox-row";
            const checkbox = document.createElement("input");
            checkbox.type = "checkbox";
            checkbox.addEventListener("change", () => {
                if (checkbox.checked) selectedHosts.add(i);
                else selectedHosts.delete(i);
            });
            row.append(checkbox, document.createTextNode(" " + h.patterns.join(", ") + " (" + h.sourceFile + ")"));
            hostsList.appendChild(row);
        });
        if (hosts.length === 0) {
            hostsList.textContent = "nenhum host encontrado";
        }
    }

    function renderKeysList() {
        keysList.innerHTML = "";
        keys.forEach((k, i) => {
            const row = document.createElement("div");
            row.className = "key-select-row";

            const includeCheckbox = document.createElement("input");
            includeCheckbox.type = "checkbox";
            const privateCheckbox = document.createElement("input");
            privateCheckbox.type = "checkbox";
            privateCheckbox.disabled = true;

            includeCheckbox.addEventListener("change", () => {
                privateCheckbox.disabled = !includeCheckbox.checked;
                if (!includeCheckbox.checked) {
                    privateCheckbox.checked = false;
                    selectedKeys.delete(i);
                } else {
                    selectedKeys.set(i, privateCheckbox.checked);
                }
                updatePrivateWarning();
            });
            privateCheckbox.addEventListener("change", () => {
                selectedKeys.set(i, privateCheckbox.checked);
                updatePrivateWarning();
            });

            const label = document.createElement("label");
            label.append(includeCheckbox, document.createTextNode(" " + (k.metadata.label || k.metadata.fingerprint)));

            const privateLabel = document.createElement("label");
            privateLabel.className = "muted";
            privateLabel.append(privateCheckbox, document.createTextNode(" incluir privada"));

            row.append(label, privateLabel);
            keysList.appendChild(row);
        });
        if (keys.length === 0) {
            keysList.textContent = "nenhuma chave encontrada";
        }
    }

    async function load() {
        try {
            [hosts, keys] = await Promise.all([App.ListHosts(), App.ListKeys()]);
            renderHostsList();
            renderKeysList();
            clearError();
        } catch (err) {
            showError(err);
        }
    }

    chooseDestBtn.addEventListener("click", async () => {
        try {
            const path = await App.ChooseExportDestination();
            if (path) destPath.value = path;
        } catch (err) {
            showError(err);
        }
    });

    exportBtn.addEventListener("click", async () => {
        successBox.hidden = true;
        if (!destPath.value) {
            showError(new Error("informe o caminho de destino do arquivo .tar.gz"));
            return;
        }

        const chosenHosts = [...selectedHosts].map((i) => hosts[i]);
        const chosenKeys = [...selectedKeys.entries()].map(([i, includePrivate]) => ({
            fingerprint: keys[i].metadata.fingerprint,
            includePrivate,
        }));
        const anyPrivate = chosenKeys.some((k) => k.includePrivate);

        let body = `${chosenHosts.length} host(s) e ${chosenKeys.length} chave(s) serão exportados para ${destPath.value}.\nSe já existir um arquivo nesse caminho, será sobrescrito.`;
        if (anyPrivate) {
            body += "\n\nATENÇÃO: o pacote incluirá chave(s) PRIVADA(S) em texto claro — armazene o arquivo resultante com segurança.";
        }

        const ok = await confirmDialog({ title: "Exportar backup?", body, danger: anyPrivate });
        if (!ok) return;

        try {
            await App.Export({
                destPath: destPath.value,
                hosts: chosenHosts,
                keys: chosenKeys,
                includeSettings: includeSettings.checked,
            });
            clearError();
            showSuccess(`Pacote salvo em ${destPath.value}.`);
        } catch (err) {
            showError(err);
        }
    });

    load();
}

function renderImportTab(root: HTMLElement) {
    root.innerHTML = `
        <div id="error" class="error" hidden></div>

        <label class="entity-form-row">Origem
            <div class="dest-row">
                <input id="src-path" placeholder="ex: C:\\backup\\keyward-backup.tar.gz"/>
                <button type="button" id="choose-src-btn">Escolher arquivo…</button>
            </div>
        </label>
        <div class="form-buttons">
            <button type="button" id="preview-btn">Pré-visualizar</button>
        </div>

        <div id="preview-result"></div>
        <div id="result-box"></div>
    `;

    const errorBox = root.querySelector("#error") as HTMLDivElement;
    const srcPath = root.querySelector("#src-path") as HTMLInputElement;
    const chooseSrcBtn = root.querySelector("#choose-src-btn") as HTMLButtonElement;
    const previewBtn = root.querySelector("#preview-btn") as HTMLButtonElement;
    const previewResult = root.querySelector("#preview-result") as HTMLDivElement;
    const resultBox = root.querySelector("#result-box") as HTMLDivElement;

    // Resolução por item — chave estável igual à usada em
    // ImportResolutionsInput (ver dto_backup.go): host sem conflito usa
    // App.HostImportKey(sourceFile, patterns); host em conflito usa
    // HostConflictDTO.Key; chave SSH sempre usa o fingerprint.
    const hostResolutions = new Map<string, "skip" | "apply">();
    const keyResolutions = new Map<string, "skip" | "overwrite" | "importRenamed">();
    let currentPreview: ImportPreviewDTO | null = null;

    function showError(err: unknown) {
        errorBox.textContent = err instanceof Error ? err.message : String(err);
        errorBox.hidden = false;
    }

    function clearError() {
        errorBox.hidden = true;
        errorBox.textContent = "";
    }

    function summaryIsDanger(): boolean {
        if (currentPreview?.settings) return true;
        for (const v of hostResolutions.values()) if (v === "apply") return true;
        for (const v of keyResolutions.values()) if (v === "overwrite") return true;
        return false;
    }

    async function renderPreview(p: ImportPreviewDTO) {
        currentPreview = p;
        hostResolutions.clear();
        keyResolutions.clear();
        previewResult.innerHTML = "";
        resultBox.innerHTML = "";

        // Hosts/chaves "a adicionar" — cherry-pick via checkbox, marcada
        // por padrão (desmarcar = Skip explícito).
        function addToAddRow(container: HTMLElement, text: string, onToggle: (included: boolean) => void) {
            const row = document.createElement("label");
            row.className = "checkbox-row";
            const checkbox = document.createElement("input");
            checkbox.type = "checkbox";
            checkbox.checked = true;
            checkbox.addEventListener("change", () => onToggle(checkbox.checked));
            row.append(checkbox, document.createTextNode(" [+] " + text));
            container.appendChild(row);
        }

        function addUnchangedRow(container: HTMLElement, text: string) {
            const row = document.createElement("div");
            row.className = "muted";
            row.textContent = "[=] " + text + " (sem mudança)";
            container.appendChild(row);
        }

        function section(title: string, count: number): HTMLElement {
            const wrap = document.createElement("div");
            const h = document.createElement("h3");
            h.textContent = `${title} (${count})`;
            wrap.appendChild(h);
            previewResult.appendChild(wrap);
            return wrap;
        }

        if (p.hostsToAdd.length > 0) {
            const wrap = section("Hosts a adicionar", p.hostsToAdd.length);
            for (const h of p.hostsToAdd) {
                const key = await App.HostImportKey(h.sourceFile, h.patterns);
                addToAddRow(wrap, h.patterns.join(", ") + " → " + h.sourceFile, (included) => {
                    hostResolutions.set(key, included ? "apply" : "skip");
                });
            }
        }
        if (p.hostsUnchanged.length > 0) {
            const wrap = section("Hosts sem mudança", p.hostsUnchanged.length);
            for (const h of p.hostsUnchanged) addUnchangedRow(wrap, h.patterns.join(", "));
        }
        if (p.hostConflicts.length > 0) {
            const wrap = section("Conflitos de host", p.hostConflicts.length);
            for (const c of p.hostConflicts) {
                hostResolutions.set(c.key, "skip");
                const row = document.createElement("div");
                row.className = "conflict-row";
                const label = document.createElement("span");
                label.textContent =
                    "[!] " +
                    c.host.patterns.join(", ") +
                    " em " +
                    c.destinationFile +
                    (c.externalPath ? " (caminho fora do home)" : "") +
                    (c.existing ? " (já existe localmente)" : "");
                const select = document.createElement("select");
                select.innerHTML = `<option value="skip">Pular</option><option value="apply">Substituir</option>`;
                select.addEventListener("change", () => {
                    hostResolutions.set(c.key, select.value as "skip" | "apply");
                });
                row.append(label, select);
                wrap.appendChild(row);
            }
        }
        if (p.keysToAdd.length > 0) {
            const wrap = section("Chaves a adicionar", p.keysToAdd.length);
            for (const k of p.keysToAdd) {
                addToAddRow(wrap, k.label || k.fingerprint, (included) => {
                    keyResolutions.set(k.fingerprint, included ? "overwrite" : "skip");
                });
                // Chave "a adicionar" sem conflito: default é aplicar
                // (equivalente a estar ausente do mapa) — só entra no
                // mapa se o usuário desmarcar (cherry-pick de exclusão).
                keyResolutions.delete(k.fingerprint);
            }
        }
        if (p.keysUnchanged.length > 0) {
            const wrap = section("Chaves sem mudança", p.keysUnchanged.length);
            for (const k of p.keysUnchanged) addUnchangedRow(wrap, k.label || k.fingerprint);
        }
        if (p.keyConflicts.length > 0) {
            const wrap = section("Conflitos de chave", p.keyConflicts.length);
            for (const c of p.keyConflicts) {
                keyResolutions.set(c.key, "skip");
                const row = document.createElement("div");
                row.className = "conflict-row";
                const label = document.createElement("span");
                const kind = c.kind === "fileNameCollision" ? "colisão de nome de arquivo" : "já registrada";
                label.textContent = "[!] " + (c.metadata.label || c.metadata.fingerprint) + ` (${kind})`;
                const select = document.createElement("select");
                select.innerHTML = `<option value="skip">Pular</option><option value="overwrite">Sobrescrever</option>`;
                if (c.kind === "fileNameCollision") {
                    select.innerHTML += `<option value="importRenamed">Importar renomeada</option>`;
                }
                select.addEventListener("change", () => {
                    keyResolutions.set(c.key, select.value as "skip" | "overwrite" | "importRenamed");
                });
                row.append(label, select);
                wrap.appendChild(row);
            }
        }
        if (p.settings) {
            const note = document.createElement("div");
            note.className = "warning-high-contrast";
            note.textContent =
                "O pacote inclui configurações (AppSettings) — serão aplicadas automaticamente se você confirmar o import (o core não permite excluir só esse item).";
            previewResult.appendChild(note);
        }

        const empty =
            p.hostsToAdd.length === 0 &&
            p.hostsUnchanged.length === 0 &&
            p.hostConflicts.length === 0 &&
            p.keysToAdd.length === 0 &&
            p.keysUnchanged.length === 0 &&
            p.keyConflicts.length === 0;

        if (empty) {
            previewResult.textContent = "Pacote vazio — nada a importar.";
        } else {
            const applyBtn = document.createElement("button");
            applyBtn.type = "button";
            applyBtn.id = "apply-import-btn";
            applyBtn.textContent = "Aplicar import";
            applyBtn.addEventListener("click", applyImport);
            previewResult.appendChild(applyBtn);
        }
    }

    function renderResult(result: ImportResultDTO) {
        resultBox.innerHTML = "";
        const h = document.createElement("h2");
        h.textContent = "Resultado";
        const summary = document.createElement("p");
        summary.textContent =
            `Hosts: ${result.hostsAdded.length} adicionados, ${result.hostsReplaced.length} substituídos, ` +
            `${result.hostsUnchanged.length} sem mudança, ${result.hostsSkipped.length} pulados. ` +
            `Chaves: ${result.keysAdded.length} adicionadas, ${result.keysUnchanged.length} sem mudança, ` +
            `${result.keysMetadataUpdated.length} metadata atualizada, ${result.keysFileOverwritten.length} arquivo sobrescrito, ` +
            `${result.keysImportedRenamed.length} renomeadas, ${result.keysSkipped.length} puladas.` +
            (result.settingsApplied ? " Configurações aplicadas." : "");
        resultBox.append(h, summary);

        if (result.errors.length > 0 || result.error) {
            const errList = document.createElement("div");
            errList.className = "error";
            const lines = [...result.errors];
            if (result.error) lines.unshift(result.error);
            errList.textContent = lines.join("\n");
            resultBox.appendChild(errList);
        }
    }

    async function applyImport() {
        if (!currentPreview) return;
        const danger = summaryIsDanger();
        let body = "Aplicar o import conforme as resoluções escolhidas em cada item?";
        if (currentPreview.settings) {
            body +=
                "\n\nATENÇÃO: o pacote inclui configurações (AppSettings), que serão aplicadas automaticamente — o core não permite excluir só esse item do import.";
        }
        const ok = await confirmDialog({ title: "Confirmar import?", body, danger });
        if (!ok) return;

        try {
            const result = await App.Import(srcPath.value, {
                hosts: Object.fromEntries(hostResolutions),
                keys: Object.fromEntries(keyResolutions),
            });
            clearError();
            renderResult(result);
        } catch (err) {
            showError(err);
        }
    }

    chooseSrcBtn.addEventListener("click", async () => {
        try {
            const path = await App.ChooseImportSource();
            if (path) srcPath.value = path;
        } catch (err) {
            showError(err);
        }
    });

    previewBtn.addEventListener("click", async () => {
        resultBox.innerHTML = "";
        if (!srcPath.value) {
            showError(new Error("informe o caminho do arquivo .tar.gz a importar"));
            return;
        }
        try {
            const preview = await App.PreviewImport(srcPath.value);
            await renderPreview(preview);
            clearError();
        } catch (err) {
            showError(err);
        }
    });
}
