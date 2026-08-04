import { App } from "../../bindings/github.com/felipehrs/keyward/internal/adapter/gui";
import { createFeedback } from "../feedback";

export function renderSettingsPage(root: HTMLElement) {
    root.innerHTML = `
        <h1>Configurações</h1>
        <div id="error" class="error" hidden></div>
        <div id="saved" class="saved" hidden></div>
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
                <button type="submit" id="submit-btn">Salvar</button>
            </div>
        </form>
    `;

    const form = root.querySelector("#settings-form") as HTMLFormElement;
    const thresholdInput = root.querySelector("#alertThresholdDays") as HTMLInputElement;
    const algorithmInput = root.querySelector("#defaultAlgorithm") as HTMLSelectElement;
    const submitBtn = root.querySelector("#submit-btn") as HTMLButtonElement;
    const { showError, showSuccess, clear } = createFeedback(root, { successSelector: "#saved" });

    async function load() {
        try {
            const settings = await App.GetSettings();
            thresholdInput.value = String(settings.alertThresholdDays);
            algorithmInput.value = settings.defaultAlgorithm;
            clear();
        } catch (err) {
            showError(err);
        }
    }

    form.addEventListener("submit", async (ev) => {
        ev.preventDefault();
        submitBtn.disabled = true;
        try {
            await App.UpdateSettings({
                alertThresholdDays: Number(thresholdInput.value),
                defaultAlgorithm: algorithmInput.value,
            });
            showSuccess("Configurações salvas.");
        } catch (err) {
            showError(err);
        } finally {
            submitBtn.disabled = false;
        }
    });

    load();
}
