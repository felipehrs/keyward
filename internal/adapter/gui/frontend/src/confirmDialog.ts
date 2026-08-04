// ConfirmDialog é o componente reutilizável pra toda ação destrutiva da GUI
// (espelha cmd/tui/confirm.go): foco default em "Cancelar" (seguro), estilo
// de alto contraste quando danger=true. app.go nunca decide isso sozinho —
// é papel do frontend nunca chamar um binding destrutivo sem passar por
// aqui antes.
//
// Construído sobre a base de <dialog> nativo de modal.ts — ver esse arquivo
// para o porquê (focus trap, Escape, restauração de foco e empilhamento
// corretos de graça, sem reimplementar nada disso aqui).
import { createDialog, mountDialog } from "./modal";

export interface ConfirmOptions {
    title: string;
    body: string;
    danger?: boolean;
}

export function confirmDialog(opts: ConfirmOptions): Promise<boolean> {
    return new Promise((resolve) => {
        const dialog = createDialog({ danger: opts.danger });

        const title = document.createElement("h3");
        title.textContent = opts.title;

        const body = document.createElement("p");
        body.textContent = opts.body; // white-space: pre-wrap no CSS preserva \n

        const actions = document.createElement("div");
        actions.className = "dialog-actions";

        const cancelBtn = document.createElement("button");
        cancelBtn.type = "button";
        cancelBtn.className = "btn-secondary";
        cancelBtn.textContent = "Cancelar";

        const confirmBtn = document.createElement("button");
        confirmBtn.type = "button";
        confirmBtn.className = "btn-primary";
        confirmBtn.textContent = "Confirmar";

        actions.append(cancelBtn, confirmBtn);
        dialog.append(title, body, actions);

        let resolved = false;
        function finish(result: boolean) {
            if (resolved) return;
            resolved = true;
            resolve(result);
        }

        cancelBtn.addEventListener("click", () => {
            finish(false);
            dialog.close();
        });
        confirmBtn.addEventListener("click", () => {
            finish(true);
            dialog.close();
        });
        // Escape (evento "cancel" nativo) e qualquer outro caminho de
        // fechamento (clique fora, etc.) caem aqui — finish() é idempotente,
        // então um close() já resolvido por clique em botão não é sobrescrito.
        dialog.addEventListener("cancel", () => finish(false));
        dialog.addEventListener("close", () => finish(false));

        mountDialog(dialog);
        cancelBtn.focus();
    });
}
