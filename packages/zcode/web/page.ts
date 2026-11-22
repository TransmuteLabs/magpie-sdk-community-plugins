import type { ModuleUI } from "@magpie/extension-sdk/ui";
import { ZcodeControl } from "./control.js";

export function mountControlPage(
  document: Document,
  backend: ModuleUI["backend"],
  confirmWrite: (message: string) => boolean | Promise<boolean>,
): {
  ready: Promise<void>;
  reload: () => Promise<void>;
  selection: () => ReturnType<ZcodeControl["snapshot"]>["selected"];
  dispose: () => void;
} {
  const element = <T extends HTMLElement>(id: string): T => {
    const value = document.getElementById(id);
    if (!value) throw new Error(`Missing control element: ${id}`);
    return value as T;
  };
  const account = element<HTMLSelectElement>("account");
  const reload = element<HTMLButtonElement>("reload");
  const plan = element<HTMLInputElement>("plan");
  const window = element<HTMLSelectElement>("reset-window");
  const status = element<HTMLParagraphElement>("status");
  const output = element<HTMLPreElement>("output");
  const nativeSession = element<HTMLInputElement>("native-session");
  const nativeCommand = element<HTMLInputElement>("native-command");
  const nativeModel = element<HTMLInputElement>("native-model");
  const nativeText = element<HTMLTextAreaElement>("native-text");
  const nativeStop = element<HTMLButtonElement>("native-stop");
  const nativeStatus = element<HTMLParagraphElement>("native-status");
  const actions = {
    executions: element<HTMLButtonElement>("executions"),
    billing: element<HTMLButtonElement>("billing"),
    resets: element<HTMLButtonElement>("resets"),
    offers: element<HTMLButtonElement>("offers"),
    checkin: element<HTMLButtonElement>("checkin"),
    claim: element<HTMLButtonElement>("claim"),
    reset: element<HTMLButtonElement>("reset"),
    resetOpportunity: element<HTMLButtonElement>("reset-opportunity"),
    resetReconcile: element<HTMLButtonElement>("reset-reconcile"),
    resetAcknowledge: element<HTMLButtonElement>("reset-ack"),
    resetResolve: element<HTMLButtonElement>("reset-resolve"),
    opportunityResolve: element<HTMLButtonElement>("opportunity-resolve"),
    nativeRun: element<HTMLButtonElement>("native-run"),
  };
  const control = new ZcodeControl(backend);
  const listeners: Array<() => void> = [];
  let pending = false;
  let disposed = false;
  let nativeActive = false;
  let nativeStopping = false;
  const cancelled = Symbol("cancelled");
  const render = () => {
    if (disposed) return;
    const state = control.snapshot();
    account.disabled = pending || state.busy;
    reload.disabled = pending || state.busy;
    plan.disabled = pending || state.busy || !state.selected;
    window.disabled = plan.disabled;
    for (const button of Object.values(actions)) button.disabled = plan.disabled;
    for (const input of [nativeSession, nativeCommand, nativeModel, nativeText]) input.disabled = plan.disabled;
    nativeStop.disabled = !nativeActive || nativeStopping || disposed;
    output.textContent = state.last === undefined ? "Пока нет данных." : JSON.stringify(state.last, null, 2);
    account.value = state.selected?.id ?? "";
  };
  const run = async (work: () => Promise<unknown>, success: string) => {
    if (pending || disposed) return;
    pending = true;
    status.textContent = "Выполняется…";
    render();
    try {
      const result = await work();
      if (!disposed) status.textContent = result === cancelled ? "Действие отменено." : success;
    } catch (error) {
      if (!disposed) {
        const message = error instanceof Error ? error.message : "Операция не подтверждена";
        status.textContent = `${message}. Повтор автоматически не выполняется. Проверьте состояние перед новой операцией.`;
      }
    } finally {
      pending = false;
      render();
    }
  };
  const load = () => run(async () => {
    const state = await control.refreshAccounts();
    if (disposed) return;
    const placeholder = document.createElement("option");
    placeholder.value = "";
    placeholder.textContent = state.accounts.length ? "Выберите счёт" : "Нет доступных счетов";
    const options = state.accounts.map((value) => {
      const option = document.createElement("option");
      option.value = value.id;
      option.textContent = value.name || value.id;
      return option;
    });
    account.replaceChildren(placeholder, ...options);
  }, "Список счетов обновлён. Выберите счёт для управления.");
  const listen = (target: HTMLElement, event: string, handler: () => void) => {
    target.addEventListener(event, handler);
    listeners.push(() => target.removeEventListener(event, handler));
  };
  listen(account, "change", () => {
    if (pending || disposed) return;
    try {
      control.select(account.value);
      status.textContent = "Счёт выбран. Загрузите нужное состояние.";
    } catch (error) {
      status.textContent = error instanceof Error ? error.message : "Счёт недоступен";
    }
    render();
  });
  listen(reload, "click", () => { void load(); });
  listen(actions.nativeRun, "click", () => {
    if (pending || disposed || !control.snapshot().selected) return;
    const selected = control.snapshot().selected!;
    const input = { sessionId: nativeSession.value, commandId: nativeCommand.value,
      model: nativeModel.value, text: nativeText.value };
    void run(async () => {
      if (!await confirmWrite(`Отправить один ввод в CLI-сессию ${input.sessionId} для счёта ${selected.id}?`) || disposed) return cancelled;
      nativeActive = true;
      nativeStopping = false;
      nativeStatus.textContent = "Ожидается подтверждённый результат команды. Остановка относится только к этой команде.";
      render();
      try {
        return await control.nativeRun(input);
      } finally {
        nativeActive = false;
        nativeStatus.textContent = "Ожидание завершено. Проверьте результат; неизвестная команда автоматически не повторяется.";
      }
    }, "Получен терминальный результат CLI-команды и подтверждена остановка процесса.");
  });
  listen(nativeStop, "click", () => {
    if (!nativeActive || nativeStopping || disposed) return;
    nativeStopping = true;
    render();
    void control.nativeStop().then(() => {
      if (!disposed) nativeStatus.textContent = "Остановка запрошена. Ожидается терминальный результат команды.";
    }).catch(() => {
      if (!disposed) nativeStatus.textContent = "Остановка не подтверждена. Повтор автоматически не выполняется.";
    });
  });
  listen(actions.executions, "click", () => { void run(() => control.read("executionList"), "Исполнения загружены."); });
  listen(actions.billing, "click", () => { void run(() => control.read("billingStatus"), "Планы загружены."); });
  listen(actions.resets, "click", () => { void run(() => control.read("resetSummary"), "Состояние сбросов и ожидающих подтверждения действий загружено."); });
  listen(actions.offers, "click", () => { void run(() => control.read("billingOffers"), "Доступные предложения загружены."); });
  listen(actions.checkin, "click", () => {
    if (pending || disposed || !control.snapshot().selected) return;
    const selected = control.snapshot().selected!;
    void run(async () => {
      if (!await confirmWrite(`Проверить предложения и запросить доступные подарочные планы для счёта ${selected.id}?`) || disposed) return cancelled;
      return control.checkin();
    }, "Ответ на check-in получен. Проверьте результат.");
  });
  listen(actions.reset, "click", () => {
    if (pending || disposed || !control.snapshot().selected) return;
    if (window.value !== "FIVE_HOUR" && window.value !== "WEEK") {
      status.textContent = "Выберите пятичасовое или недельное окно.";
      return;
    }
    const selected = control.snapshot().selected!;
    const resetWindow = window.value;
    const label = resetWindow === "FIVE_HOUR" ? "пятичасового" : "недельного";
    void run(async () => {
      if (!await confirmWrite(`Потратить один сброс ${label} лимита для счёта ${selected.id}?`) || disposed) return cancelled;
      return control.reset(resetWindow);
    }, "Ответ на сброс получен. Проверьте результат.");
  });
  listen(actions.claim, "click", () => {
    if (pending || disposed || !control.snapshot().selected) return;
    const planId = plan.value;
    if (!planId || planId.trim() !== planId || planId.length > 128 || /[\u0000-\u001f\u007f]/u.test(planId)) {
      status.textContent = "Введите точный ID плана из состояния планов.";
      return;
    }
    const selected = control.snapshot().selected!;
    void run(async () => {
      if (!await confirmWrite(`Запросить план ${planId} для счёта ${selected.id}?`) || disposed) return cancelled;
      return control.claim(planId);
    }, "Ответ на запрос плана получен. Проверьте результат.");
  });
  const resetControls: Array<{
    button: HTMLButtonElement;
    action: Parameters<ZcodeControl["resetControl"]>[0];
    confirmation: string;
  }> = [
    { button: actions.resetOpportunity, action: "opportunity", confirmation: "Запросить возможность сброса лимита" },
    { button: actions.resetReconcile, action: "reconcile", confirmation: "Сверить ожидающий сброс с историей без повторной траты" },
    { button: actions.resetAcknowledge, action: "ack", confirmation: "Отметить историю сбросов прочитанной" },
    { button: actions.resetResolve, action: "resolve",
      confirmation: "Сброс уже мог быть потрачен. Снять локальную блокировку неизвестного результата без повторной траты" },
    { button: actions.opportunityResolve, action: "opportunity.resolve",
      confirmation: "Сброс уже мог быть выдан. Снять локальную блокировку неизвестной выдачи, сохранив паузу перед новым запросом" },
  ];
  for (const entry of resetControls) {
    listen(entry.button, "click", () => {
      if (pending || disposed || !control.snapshot().selected) return;
      const selected = control.snapshot().selected!;
      void run(async () => {
        if (!await confirmWrite(`${entry.confirmation} для счёта ${selected.id}?`) || disposed) return cancelled;
        return control.resetControl(entry.action);
      }, "Ответ получен. Проверьте состояние сбросов перед следующим действием.");
    });
  }
  render();
  return {
    ready: load(),
    reload: load,
    selection: () => pending || disposed ? undefined : control.snapshot().selected,
    dispose() {
      disposed = true;
      for (const remove of listeners) remove();
    },
  };
}
