// createFeedback centraliza o trio showError/showSuccess/clear que cada
// página reimplementava (cópias quase idênticas espalhadas por
// hostsPage/keysPage/backupPage×2/settingsPage). Espera um `<div id="error">`
// dentro de `root`; o box de sucesso é opcional (settingsPage usa "#saved"
// em vez de "#success" — daí o parâmetro).
export interface Feedback {
    showError: (err: unknown) => void;
    showSuccess: (message: string) => void;
    clear: () => void;
}

export function createFeedback(root: ParentNode, opts?: { successSelector?: string }): Feedback {
    const errorBox = root.querySelector<HTMLElement>("#error");
    const successBox = root.querySelector<HTMLElement>(opts?.successSelector ?? "#success");

    function showError(err: unknown) {
        if (errorBox) {
            errorBox.textContent = err instanceof Error ? err.message : String(err);
            errorBox.hidden = false;
        }
        if (successBox) successBox.hidden = true;
    }

    function showSuccess(message: string) {
        if (successBox) {
            successBox.textContent = message;
            successBox.hidden = false;
        }
        if (errorBox) errorBox.hidden = true;
    }

    function clear() {
        if (errorBox) {
            errorBox.hidden = true;
            errorBox.textContent = "";
        }
    }

    return { showError, showSuccess, clear };
}
