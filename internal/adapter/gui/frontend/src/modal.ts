// Base compartilhada de <dialog> nativo, usada por confirmDialog.ts e por
// formModal() abaixo. O <dialog>+showModal() dá de graça o que a versão
// anterior (um <div class="confirm-overlay"> manual) tinha que implementar
// à mão: focus trap, Escape via evento "cancel" (sem listener em
// document), restauração de foco no elemento que abriu o modal ao fechar,
// e empilhamento correto quando um modal abre outro por cima (ex.: o
// confirm de sobrescrever chave, aberto de dentro do formulário de gerar
// chave).

let dialogSeq = 0;

interface DialogOptions {
    danger?: boolean;
    wide?: boolean;
}

// Cria um <dialog> vazio, ainda fora do DOM. Quem chama popula o conteúdo e
// então usa mountDialog() pra exibir. Clique fora da caixa (na área do
// ::backdrop, que entrega o clique com target === dialog) fecha; o "close"
// nativo (disparado por close(), Escape, ou esse clique) sempre remove o
// elemento do DOM — nenhum caminho de fechamento deixa lixo.
export function createDialog(opts: DialogOptions): HTMLDialogElement {
    const dialog = document.createElement("dialog");
    dialog.className =
        "app-dialog" + (opts.danger ? " app-dialog-danger" : "") + (opts.wide ? " app-dialog-wide" : "");
    dialog.addEventListener("click", (ev) => {
        if (ev.target === dialog) dialog.close();
    });
    dialog.addEventListener("close", () => dialog.remove());
    return dialog;
}

export function mountDialog(dialog: HTMLDialogElement): void {
    document.body.append(dialog);
    dialog.showModal();
}

// Lançado por dentro de um onSubmit de formModal pra fechar/reabortar o
// fluxo sem que o modal trate isso como sucesso (fecha) nem como erro
// (mostra mensagem) — caso real: usuário cancela o confirm de "sobrescrever
// chave existente" que abre por cima do formulário de gerar chave; o
// formulário deve continuar aberto, com os valores preenchidos, sem
// nenhuma mensagem de erro.
export class ModalCancelled extends Error {}

export interface FieldSpec {
    name: string;
    label: string;
    type?: "text" | "password" | "number" | "date" | "select";
    value?: string;
    placeholder?: string;
    required?: boolean;
    options?: { value: string; label: string }[]; // obrigatório se type === "select"
    // Reavaliado a cada mudança em qualquer campo do formulário. Um campo
    // invisível (`hidden`) fica automaticamente fora da validação nativa do
    // form, mesmo se `required` — comportamento padrão do HTML, sem código
    // extra necessário aqui.
    visibleWhen?: (values: Record<string, string>) => boolean;
}

export interface FormModalOptions {
    title: string;
    fields: FieldSpec[];
    submitLabel?: string;
    danger?: boolean;
    wide?: boolean;
    onSubmit: (values: Record<string, string>) => Promise<void>;
}

// formModal nunca fecha "esperando" um valor de volta — ele recebe
// onSubmit, que PODE falhar. Se onSubmit lançar, o erro aparece dentro do
// próprio modal e ele continua aberto com os valores preservados; só
// fecha quando onSubmit resolve. Devolve true se chegou a submeter com
// sucesso, false se foi cancelado (botão Cancelar, Escape, ou clique fora).
export function formModal(opts: FormModalOptions): Promise<boolean> {
    return new Promise((resolve) => {
        const dialog = createDialog({ danger: opts.danger, wide: opts.wide });
        const titleId = `dialog-title-${++dialogSeq}`;
        dialog.setAttribute("aria-labelledby", titleId);

        const titleEl = document.createElement("h3");
        titleEl.id = titleId;
        titleEl.textContent = opts.title;

        const errorBox = document.createElement("div");
        errorBox.className = "error";
        errorBox.hidden = true;

        const form = document.createElement("form");
        form.className = "entity-form modal-form";
        // Sem method="dialog" de propósito — queremos controlar o fechamento
        // manualmente, só depois que onSubmit resolver.

        const inputs = new Map<string, HTMLInputElement | HTMLSelectElement>();
        const fieldWraps = new Map<string, HTMLElement>();

        for (const f of opts.fields) {
            const wrap = document.createElement("label");
            wrap.append(document.createTextNode(f.label + " "));

            let input: HTMLInputElement | HTMLSelectElement;
            if (f.type === "select") {
                const select = document.createElement("select");
                for (const o of f.options ?? []) {
                    const option = document.createElement("option");
                    option.value = o.value;
                    option.textContent = o.label;
                    select.appendChild(option);
                }
                input = select;
            } else {
                const inp = document.createElement("input");
                inp.type = f.type ?? "text";
                if (f.placeholder) inp.placeholder = f.placeholder;
                input = inp;
            }
            input.id = `field-${f.name}-${dialogSeq}`;
            if (f.value !== undefined) input.value = f.value;
            if (f.required) input.required = true;

            wrap.appendChild(input);
            form.appendChild(wrap);
            inputs.set(f.name, input);
            fieldWraps.set(f.name, wrap);
        }

        function collectValues(): Record<string, string> {
            const values: Record<string, string> = {};
            for (const [name, input] of inputs) values[name] = input.value;
            return values;
        }

        function updateVisibility() {
            const values = collectValues();
            for (const f of opts.fields) {
                if (f.visibleWhen) {
                    fieldWraps.get(f.name)!.hidden = !f.visibleWhen(values);
                }
            }
        }
        for (const input of inputs.values()) {
            input.addEventListener("input", updateVisibility);
            input.addEventListener("change", updateVisibility);
        }
        updateVisibility();

        const actions = document.createElement("div");
        actions.className = "dialog-actions";
        const cancelBtn = document.createElement("button");
        cancelBtn.type = "button";
        cancelBtn.className = "btn-secondary";
        cancelBtn.textContent = "Cancelar";
        const submitBtn = document.createElement("button");
        submitBtn.type = "submit";
        submitBtn.className = "btn-primary";
        submitBtn.textContent = opts.submitLabel ?? "Salvar";
        actions.append(cancelBtn, submitBtn);
        form.appendChild(actions);

        dialog.append(titleEl, errorBox, form);

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
        dialog.addEventListener("cancel", () => finish(false));
        dialog.addEventListener("close", () => finish(false));

        form.addEventListener("submit", async (ev) => {
            ev.preventDefault();
            errorBox.hidden = true;
            // Guarda contra duplo-submit: metadata.json não tem lock entre
            // processos/chamadas concorrentes (risco aceito, ver CLAUDE.md/
            // spec seção 6) — desabilitar o botão durante a chamada em voo
            // era uma mitigação já prevista no plano original da GUI, nunca
            // implementada até agora.
            submitBtn.disabled = true;
            cancelBtn.disabled = true;
            try {
                await opts.onSubmit(collectValues());
                finish(true);
                dialog.close();
            } catch (err) {
                if (err instanceof ModalCancelled) {
                    // Fluxo interno cancelado (ex.: usuário recusou
                    // sobrescrever) — continua aberto, sem mensagem de erro.
                    return;
                }
                errorBox.textContent = err instanceof Error ? err.message : String(err);
                errorBox.hidden = false;
            } finally {
                submitBtn.disabled = false;
                cancelBtn.disabled = false;
            }
        });

        mountDialog(dialog);
        const firstField = opts.fields[0];
        if (firstField) inputs.get(firstField.name)?.focus();
    });
}
