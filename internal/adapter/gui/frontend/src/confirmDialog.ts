// ConfirmDialog é o componente único e reutilizável pra toda ação
// destrutiva da GUI (espelha cmd/tui/confirm.go): foco default em
// "Cancelar" (seguro), estilo de alto contraste quando danger=true. app.go
// nunca decide isso sozinho — é papel do frontend nunca chamar um binding
// destrutivo sem passar por aqui antes.
export interface ConfirmOptions {
    title: string;
    body: string;
    danger?: boolean;
}

export function confirmDialog(opts: ConfirmOptions): Promise<boolean> {
    return new Promise((resolve) => {
        const overlay = document.createElement("div");
        overlay.className = "confirm-overlay";

        const box = document.createElement("div");
        box.className = opts.danger ? "confirm-box confirm-danger" : "confirm-box";

        const title = document.createElement("h3");
        title.textContent = opts.title;

        const body = document.createElement("p");
        body.textContent = opts.body;

        const actions = document.createElement("div");
        actions.className = "confirm-actions";

        const cancelBtn = document.createElement("button");
        cancelBtn.type = "button";
        cancelBtn.className = "confirm-cancel";
        cancelBtn.textContent = "Cancelar";

        const confirmBtn = document.createElement("button");
        confirmBtn.type = "button";
        confirmBtn.className = "confirm-confirm";
        confirmBtn.textContent = "Confirmar";

        function onKeyDown(ev: KeyboardEvent) {
            if (ev.key === "Escape") close(false);
        }

        function close(result: boolean) {
            overlay.remove();
            document.removeEventListener("keydown", onKeyDown);
            resolve(result);
        }

        cancelBtn.addEventListener("click", () => close(false));
        confirmBtn.addEventListener("click", () => close(true));
        overlay.addEventListener("click", (ev) => {
            if (ev.target === overlay) close(false);
        });
        document.addEventListener("keydown", onKeyDown);

        actions.append(cancelBtn, confirmBtn);
        box.append(title, body, actions);
        overlay.append(box);
        document.body.append(overlay);

        cancelBtn.focus();
    });
}
