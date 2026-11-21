import { createAccountWorkflow, createAuthLinkPresentation, ui } from "./sdk-ui.js";
import { mountHistory } from "./history.js";

export function mount(document: Document) {
  const element = <T extends HTMLElement>(id: string): T => {
    const found = document.getElementById(id);
    if (!found) throw new Error("OpenCode page element is unavailable");
    return found as T;
  };
  const account = element<HTMLSelectElement>("account");
  const provider = element<HTMLSelectElement>("provider");
  const method = element<HTMLSelectElement>("method");
  const key = element<HTMLInputElement>("key");
  const status = element<HTMLParagraphElement>("status");
  const code = element<HTMLParagraphElement>("code");
  const authLink = createAuthLinkPresentation({
    container: element("auth-address"),
    input: element<HTMLInputElement>("auth-url"),
    select: element<HTMLButtonElement>("auth-select"),
  });
  const buttons = Object.fromEntries(["reload", "inspect", "signin", "reauthorize", "poll", "cancel"].map(
    id => [id, element<HTMLButtonElement>(id)],
  ));
  const workflows = {
    go: createAccountWorkflow("account.info", "signin"),
    zen: createAccountWorkflow("zen.account.info", "zen.signin"),
    public: createAccountWorkflow("public.account.info", "public.signin"),
  };
  let workflow = workflows.go;
  let busy = 0, disposed = false;
  const active = () => ["waiting", "pending"].includes(workflow.snapshot.flow?.state ?? "");
  const render = () => {
    if (disposed) return;
    const snapshot = workflow.snapshot;
    account.replaceChildren(new Option("Выберите счёт", ""),
      ...snapshot.accounts.map(value => new Option(`${value.id} · ${value.generation}`, value.id)));
    account.value = snapshot.selection?.id ?? "";
    account.disabled = busy > 0 || active();
    provider.disabled = method.disabled = key.disabled = busy > 0 || active();
    key.hidden = element("key-label").hidden = method.value !== "key";
    buttons.reload.disabled = busy > 0 || active();
    buttons.inspect.disabled = buttons.reauthorize.disabled = busy > 0 || active() || snapshot.selection === null;
    buttons.signin.disabled = busy > 0 || active();
    buttons.poll.disabled = busy > 0 || !active();
    buttons.cancel.disabled = !active();
    code.textContent = active() ? snapshot.flow?.userCode ?? "" : "";
    authLink.update(snapshot.flow);
  };
  const run = async (action: () => Promise<unknown>) => {
    if (disposed) return;
    busy++;
    render();
    try {
      await action();
      if (disposed) return;
      const flow = workflow.snapshot.flow;
      if (flow?.state === "complete") status.textContent = "Вход завершён. Выберите счёт заново.";
      else if (active()) status.textContent = `Подтвердите код и проверяйте не раньше чем через ${Math.ceil((flow?.retryAfterMs ?? 1000) / 1000)} с.`;
      else status.textContent = workflow.snapshot.stale ? "Счёт изменился. Обновите список и выберите его заново." : "Готово.";
    } catch {
      if (!disposed) status.textContent = "Действие не выполнено. Проверьте список счетов или состояние входа.";
    } finally {
      busy--;
      render();
    }
  };
  const listeners: Array<() => void> = [];
  const listen = (id: string, action: () => Promise<unknown>) => {
    const handler = () => { void run(action); };
    buttons[id].addEventListener("click", handler);
    listeners.push(() => buttons[id].removeEventListener("click", handler));
  };
  const selected = () => {
    try { workflow.select(account.value || null); }
    catch { status.textContent = "Счёт отсутствует в актуальном списке."; }
    render();
  };
  account.addEventListener("change", selected);
  listeners.push(() => account.removeEventListener("change", selected));
  listen("reload", () => workflow.refresh());
  listen("inspect", () => workflow.call());
  const start = (reauthorize = false) => {
    const input = method.value === "key" ? { input: { key: key.value } } : {};
    key.value = "";
    return workflow.start({ method: method.value, ...input }, reauthorize);
  };
  listen("signin", () => start());
  listen("reauthorize", () => start(true));
  listen("poll", () => workflow.finish());
  listen("cancel", () => workflow.cancel());
  const changedProvider = () => {
    if (busy > 0 || active()) return;
    key.value = "";
    workflow.select(null);
    workflow = workflows[provider.value as keyof typeof workflows] ?? workflows.go;
    const methods = provider.value === "public" ? [new Option("Публичный доступ", "public")] :
      [new Option("Код устройства", "device"), ...(provider.value === "zen" ? [new Option("Сервисный ключ", "key")] : [])];
    method.replaceChildren(...methods);
    method.value = methods[0].value;
    void run(() => workflow.refresh());
  };
  provider.addEventListener("change", changedProvider);
  method.addEventListener("change", render);
  listeners.push(() => provider.removeEventListener("change", changedProvider), () => method.removeEventListener("change", render));
  const ready = ui.ready.then(() => run(() => workflow.refresh()));
  return {
    ready, workflow,
    dispose() {
      disposed = true;
      key.value = "";
      authLink.dispose();
      for (const remove of listeners) remove();
    },
  };
}

if (typeof document !== "undefined") {
  const page = document.getElementById("account") ? mount(document) : undefined;
  const history = document.getElementById("history-panel") ? mountHistory(document) : undefined;
  window.addEventListener("pagehide", () => { page?.dispose(); history?.dispose(); }, { once: true });
}
