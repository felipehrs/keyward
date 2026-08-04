import { App } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui";

export function renderSettingsPage(root: HTMLElement) {
    root.innerHTML = `
        <h1>Configurações</h1>
        <div id="error" class="error" hidden></div>
        <div id="saved" class="saved" hidden>Configurações salvas.</div>
        <form id="settings-form" class="entity-form">
            <label>Alertar expiração com quantos dias de antecedência
                <input id="alertThresholdDays" type="number" min="0" required/>
            </label>
            <label>Algoritmo padrão
                <select id="defaultAlgorithm">
                    <option value="ed25519">ed25519</option>
                    <option value="rsa">rsa</option>
                </select>
            </label>
            <div class="form-buttons">
                <button type="submit">Salvar</button>
            </div>
        </form>
    `;

    const errorBox = root.querySelector("#error") as HTMLDivElement;
    const savedBox = root.querySelector("#saved") as HTMLDivElement;
    const form = root.querySelector("#settings-form") as HTMLFormElement;
    const thresholdInput = root.querySelector("#alertThresholdDays") as HTMLInputElement;
    const algorithmInput = root.querySelector("#defaultAlgorithm") as HTMLSelectElement;

    function showError(err: unknown) {
        errorBox.textContent = err instanceof Error ? err.message : String(err);
        errorBox.hidden = false;
        savedBox.hidden = true;
    }

    function clearError() {
        errorBox.hidden = true;
        errorBox.textContent = "";
    }

    async function load() {
        try {
            const settings = await App.GetSettings();
            thresholdInput.value = String(settings.alertThresholdDays);
            algorithmInput.value = settings.defaultAlgorithm;
            clearError();
        } catch (err) {
            showError(err);
        }
    }

    form.addEventListener("submit", async (ev) => {
        ev.preventDefault();
        savedBox.hidden = true;
        try {
            await App.UpdateSettings({
                alertThresholdDays: Number(thresholdInput.value),
                defaultAlgorithm: algorithmInput.value,
            });
            clearError();
            savedBox.hidden = false;
        } catch (err) {
            showError(err);
        }
    });

    load();
}
