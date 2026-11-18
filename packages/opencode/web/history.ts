import { ui } from "./sdk-ui.js";
import type { HistoryInspection, HistoryRoot, HistoryTranscript, SessionHistory } from "./sdk-ui.js";

export function mountHistory(document: Document, history: SessionHistory = ui.history) {
  const element = <T extends HTMLElement>(id: string): T => {
    const found = document.getElementById(id);
    if (!found) throw new Error("History view element is unavailable");
    return found as T;
  };
  const status = element("history-status");
  const roots = element("history-roots");
  const sessions = element("history-sessions");
  const transcript = element("history-transcript");
  const media = element("history-media");
  const buttons = Object.fromEntries(["refresh", "access", "release", "list", "more"].map(
    name => [name, element<HTMLButtonElement>("history-" + name)],
  ));
  let inspection: HistoryInspection | null = null;
  let root: HistoryRoot | null = null;
  let cursor: string | undefined;
  let generation = 0, busy = false, refreshing = false, supported = false, disposed = false;
  let visible = ui.visible, timer: ReturnType<typeof setTimeout> | undefined;
  const source = "opencode-history";
  const clearContent = () => {
    cursor = undefined;
    sessions.replaceChildren();
    transcript.replaceChildren();
    media.replaceChildren();
  };
  const stateText = () => {
    if (!supported) return "Просмотр истории недоступен в этой версии Magpie.";
    if (!inspection) return "Не удалось проверить доступ к истории.";
    if (inspection.ownOff) return "Локальная история выключена режимом шлюза.";
    if (!root) return "Выберите доступное хранилище истории.";
    if (!root.consented && !inspection.readAllowed) return "Нужно разрешить чтение OpenCode и отдельно выбранное хранилище.";
    if (!root.consented) return "Чтение OpenCode разрешено. Выбранное хранилище ещё не делегировано.";
    if (!inspection.readAllowed) return "Хранилище делегировано, но чтение OpenCode не разрешено.";
    return "Доступ разрешён. Загрузите список сессий.";
  };
  const render = () => {
    const inactive = disposed || !visible || busy || !supported;
    buttons.refresh.disabled = inactive || refreshing;
    buttons.access.disabled = inactive || !root?.available || inspection?.ownOff === true ||
      (root?.consented === true && inspection?.readAllowed === true);
    buttons.release.disabled = inactive || !root?.consentRef;
    buttons.list.disabled = buttons.more.disabled = inactive || !root?.consented || !inspection?.readAllowed;
    buttons.more.hidden = !cursor;
    for (const button of roots.querySelectorAll("button")) button.disabled = inactive || button.dataset.available === "false";
    for (const button of sessions.querySelectorAll("button")) button.disabled = inactive;
    for (const button of media.querySelectorAll("button")) button.disabled = inactive;
  };
  const invalidate = () => {
    generation++;
    clearContent();
  };
  const drawRoots = () => {
    roots.replaceChildren();
    for (const candidate of inspection?.roots ?? []) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = candidate.slot + (candidate.available ? "" : " — отсутствует");
      button.disabled = !candidate.available;
      button.dataset.available = String(candidate.available);
      button.setAttribute("aria-pressed", String(candidate.rootRef === root?.rootRef));
      button.addEventListener("click", () => {
        if (busy || disposed || !visible || !candidate.available) return;
        invalidate();
        root = candidate;
        drawRoots();
        status.textContent = stateText();
        render();
      });
      roots.append(button);
    }
  };
  const applyInspection = (next: HistoryInspection | null) => {
    if (next?.viewRef !== inspection?.viewRef) {
      invalidate();
      const previous = root?.rootRef;
      inspection = next;
      root = next?.roots.find(value => value.rootRef === previous && value.available) ?? null;
      drawRoots();
    }
    status.textContent = stateText();
    render();
  };
  const revalidate = async () => {
    if (disposed || !visible || !supported || refreshing) return;
    refreshing = true;
    const current = generation;
    try {
      const next = await history.inspect(source);
      if (!disposed && visible && generation === current) applyInspection(next);
    } catch {
      if (!disposed && visible && generation === current) applyInspection(null);
    } finally {
      refreshing = false;
      render();
    }
  };
  const schedule = () => {
    clearTimeout(timer);
    if (disposed || !visible || !supported) return;
    timer = setTimeout(async () => { await revalidate(); schedule(); }, 2000);
  };
  const run = async (action: (current: number) => Promise<void>) => {
    if (disposed || !visible || busy || !supported) return;
    busy = true;
    const current = generation;
    render();
    try {
      await action(current);
    } catch {
      if (!disposed && visible && current === generation) {
        invalidate();
        status.textContent = "Выбор истории изменился или чтение недоступно. Обновите доступ и список.";
      }
    } finally {
      busy = false;
      render();
    }
  };
  const active = (current: number) => !disposed && visible && current === generation;
  const selection = () => {
    if (!inspection || !root?.rootRef) throw new Error("Select an inspected history root");
    return { source, viewRef: inspection.viewRef, rootRef: root.rootRef };
  };
  const drawTranscript = (value: HistoryTranscript, current: number) => {
    transcript.replaceChildren();
    media.replaceChildren();
    for (const part of value.parts) {
      const section = document.createElement("section");
      const heading = document.createElement("p");
      heading.textContent = [part.role, part.kind, part.model].filter(Boolean).join(" · ");
      const text = document.createElement("pre");
      text.textContent = part.text;
      section.append(heading, text);
      transcript.append(section);
    }
    if (value.cut) {
      const cut = document.createElement("p");
      cut.textContent = "Показана ограниченная часть истории.";
      transcript.append(cut);
    }
    for (const descriptor of value.media) {
      const section = document.createElement("section");
      const label = document.createElement("p");
      label.textContent = `${descriptor.name || "Вложение"} · ${descriptor.mime} · ${descriptor.size} байт`;
      section.append(label);
      const raster = ["image/png", "image/jpeg", "image/gif", "image/webp"].includes(descriptor.mime);
      if (!raster && descriptor.mime !== "text/plain") {
        const unsupported = document.createElement("p");
        unsupported.textContent = "Предпросмотр этого типа вложения не поддерживается.";
        section.append(unsupported);
      } else {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = "Показать вложение";
        button.addEventListener("click", () => {
          if (!active(current)) return;
          void run(async current => {
            const selected = selection();
            const result = await history.media({ source, viewRef: selected.viewRef, sessionRef: value.sessionRef,
              readRef: value.readRef, mediaRef: descriptor.mediaRef });
            if (!active(current)) return;
            if (result.mime !== descriptor.mime || result.data.byteLength !== descriptor.size || result.data.byteLength > (8 << 20)) {
              throw new Error("Selected media changed");
            }
            if (raster) {
              const image = document.createElement("img");
              const bytes = new Uint8Array(result.data);
              let binary = "";
              for (let offset = 0; offset < bytes.length; offset += 8192) binary += String.fromCharCode(...bytes.subarray(offset, offset + 8192));
              image.src = `data:${result.mime};base64,${btoa(binary)}`;
              image.alt = descriptor.name || "Вложение OpenCode";
              image.style.maxWidth = "100%";
              section.append(image);
            } else {
              const text = document.createElement("pre");
              text.textContent = new TextDecoder().decode(result.data);
              section.append(text);
            }
            button.remove();
          });
        });
        section.append(button);
      }
      media.append(section);
    }
  };
  const load = async (current: number, more = false) => {
    const selected = selection();
    const page = await history.list({ ...selected, limit: 64, ...(more && cursor ? { cursor } : {}) });
    if (!active(current)) return;
    if (!more) {
      sessions.replaceChildren();
      transcript.replaceChildren();
      media.replaceChildren();
    }
    cursor = page.nextCursor;
    for (const entry of page.entries) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = entry.title || "Сессия OpenCode";
      button.addEventListener("click", () => {
        void run(async current => {
          transcript.replaceChildren();
          media.replaceChildren();
          const value = await history.read({ source, viewRef: selected.viewRef, sessionRef: entry.sessionRef });
          if (active(current)) drawTranscript(value, current);
        });
      });
      sessions.append(button);
    }
    status.textContent = page.entries.length ? "Выберите сессию." : "В выбранном хранилище нет сессий.";
  };
  const listeners: Array<() => void> = [];
  const listen = (name: string, action: (current: number) => Promise<void>) => {
    const handler = () => { void run(action); };
    buttons[name].addEventListener("click", handler);
    listeners.push(() => buttons[name].removeEventListener("click", handler));
  };
  listen("refresh", async () => { await revalidate(); });
  listen("access", async current => {
    const result = await history.requestAccess(selection());
    if (!active(current)) return;
    applyInspection(result.inspection);
    status.textContent = result.state === "canceled" ? "Запрос доступа отменён." :
      result.state === "ready" ? stateText() : `Доступ не завершён. ${stateText()}`;
  });
  listen("release", async current => {
    if (!root?.consentRef) return;
    const selected = selection();
    const result = await history.release({ source, viewRef: selected.viewRef, consentRef: root.consentRef });
    if (!active(current)) return;
    applyInspection(result.inspection);
    status.textContent = result.state === "released" ? "Делегирование этого хранилища отозвано. Разрешение чтения OpenCode сохранено." : stateText();
  });
  listen("list", current => load(current));
  listen("more", current => load(current, true));
  ui.onVisibility(next => {
    if (disposed || visible === next) return;
    visible = next;
    invalidate();
    clearTimeout(timer);
    if (visible) void revalidate().finally(schedule);
    render();
  });
  const ready = ui.ready.then(() => history.ready).then(async enabled => {
    if (disposed) return;
    visible = ui.visible;
    supported = enabled;
    status.textContent = stateText();
    if (supported) await revalidate();
    schedule();
    render();
  }).catch(() => { if (!disposed) { supported = false; status.textContent = stateText(); render(); } });
  render();
  return {
    ready,
    dispose() {
      disposed = true;
      invalidate();
      clearTimeout(timer);
      for (const remove of listeners) remove();
    },
  };
}
