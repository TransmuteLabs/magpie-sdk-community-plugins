import type { AccountSelection } from "@magpie/extension-sdk/ui";
import { createAccountWorkflow } from "./sdk-ui.js";
import { bindings } from "./control.js";

export function mountAuthentication(
  document: Document,
  control: {
    selection: () => AccountSelection | undefined;
    canAuthenticate: () => boolean;
    reload: () => Promise<void>;
    lockAuthentication: (locked: boolean) => void;
  },
): { dispose: () => void } {
  const element = <T extends HTMLElement>(id: string): T => {
    const value = document.getElementById(id);
    if (!value) throw new Error(`Missing authentication element: ${id}`);
    return value as T;
  };
  const start = element<HTMLButtonElement>("signin");
  const renew = element<HTMLButtonElement>("reauthorize");
  const poll = element<HTMLButtonElement>("auth-poll");
  const cancel = element<HTMLButtonElement>("auth-cancel");
  const site = element<HTMLSelectElement>("auth-site");
  const kind = element<HTMLSelectElement>("auth-kind");
  const status = element<HTMLParagraphElement>("auth-status");
  const link = element<HTMLAnchorElement>("auth-link");
  const fields = ["organization", "project", "product", "apiKeyId"] as const;
  const inputs = fields.map(name => [name, element<HTMLInputElement>("auth-" + name)] as const);
  const workflow = createAccountWorkflow(bindings.executionList, "page.signin");
  const listeners: Array<() => void> = [];
  let working = 0, disposed = false, failed = false;
  let startDispatched = false;
  const active = () => {
    const flow = workflow.snapshot.flow;
    return flow !== null && (flow.state === "waiting" || flow.state === "pending");
  };
  const render = () => {
    if (disposed) return;
    const flow = workflow.snapshot.flow;
    const locked = working > 0 || active();
    control.lockAuthentication(locked);
    start.disabled = renew.disabled = site.disabled = kind.disabled = locked || failed;
    for (const [, input] of inputs) input.disabled = locked || failed;
    poll.disabled = working > 0 || !active();
    cancel.disabled = !active();
    link.hidden = !active() || !flow?.url;
    link.removeAttribute("href");
    if (active() && flow?.url) {
      try {
        const address = new URL(flow.url);
        if (address.protocol !== "https:" || address.username || address.password) throw new Error("Invalid link");
        link.href = address.href;
      } catch {
        link.hidden = true;
        status.textContent = "Ссылка для входа недоступна. Отмените вход и проверьте состояние.";
      }
    }
  };
  const run = async (operation: () => Promise<unknown>) => {
    if (disposed) return;
    working++;
    render();
    try {
      await operation();
      if (disposed) return;
      const flow = workflow.snapshot.flow;
      if (flow?.state === "complete") {
        control.lockAuthentication(false);
        await control.reload();
        status.textContent = "Вход завершён. Выберите счёт заново.";
      } else if (flow?.state === "waiting" || flow?.state === "pending") {
        status.textContent = `Откройте ссылку и подтвердите вход. Проверяйте не раньше чем через ${Math.ceil((flow.retryAfterMs ?? 1000) / 1000)} с.`;
      } else {
        status.textContent = `Вход завершён со статусом: ${flow?.state ?? "unknown"}. Автоматического повтора нет.`;
      }
    } catch {
      failed = startDispatched && !workflow.snapshot.flow;
      status.textContent = "Ответ не подтверждён. Обмен автоматически не повторяется. Проверьте состояние входа или откройте страницу заново.";
    } finally {
      working--;
      render();
    }
  };
  const begin = (reauthorize: boolean) => {
    if (working || active() || failed || disposed) return;
    if (!control.canAuthenticate()) {
      status.textContent = "Дождитесь завершения текущего действия.";
      return;
    }
    const selected = control.selection();
    if (reauthorize && !selected) {
      status.textContent = "Сначала выберите сохранённый счёт.";
      return;
    }
    if (!["zai", "bigmodel", "zai_cn"].includes(site.value) || !["auto", "individual", "team", "start"].includes(kind.value)) {
      status.textContent = "Выберите поддерживаемый сайт и тип счёта.";
      return;
    }
    const input: Record<string, string> = { site: site.value === "zai_cn" ? "bigmodel" : site.value, kind: kind.value };
    if (site.value === "zai_cn") input.loginProvider = "zai_cn";
    for (const [name, field] of inputs) if (field.value) input[name] = field.value;
    startDispatched = false;
    void run(async () => {
      if (reauthorize) {
        await workflow.refresh();
        const current = workflow.select(selected!.id);
        if (current?.expectedGeneration !== selected!.expectedGeneration) {
          workflow.select(null);
          throw new Error("Account generation changed; explicit reselection is required");
        }
      }
      if (!disposed) {
        startDispatched = true;
        await workflow.start({ method: "browser", ...(reauthorize ? {} : { input }) }, reauthorize);
      }
    });
  };
  const listen = (target: HTMLElement, handler: () => void) => {
    target.addEventListener("click", handler);
    listeners.push(() => target.removeEventListener("click", handler));
  };
  listen(start, () => begin(false));
  listen(renew, () => begin(true));
  listen(poll, () => { if (!working && active()) void run(() => workflow.finish()); });
  listen(cancel, () => { if (active()) void run(() => workflow.cancel()); });
  render();
  return {
    dispose() {
      disposed = true;
      for (const remove of listeners) remove();
    },
  };
}
