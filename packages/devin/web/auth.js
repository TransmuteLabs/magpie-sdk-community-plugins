import { createAuthLinkPresentation } from "./sdk-ui.js";

export function mountAuthentication(document, magpie) {
  const element = id => document.getElementById(id);
  const signin = element("signin");
  const finish = element("finish");
  const cancel = element("cancel");
  const status = element("status");
  const address = createAuthLinkPresentation({
    container: element("auth-address"), input: element("auth-url"), select: element("auth-select"),
  });
  const binding = "page.auth.devin";
  let active = null;
  let busy = false;
  let disposed = false;
  const pending = view => Boolean(view && ["waiting", "pending"].includes(view.state));
  const controls = () => {
    signin.disabled = busy || pending(active);
    finish.disabled = cancel.disabled = busy || !pending(active);
  };
  const present = view => {
    active = pending(view) ? view : null;
    address.update(view);
    status.textContent = view?.state === "complete" ? `Signed in: ${view.account.id}`
      : pending(view) ? "Authorize in your browser, then check sign-in."
      : view ? `Sign-in ${view.state}.` : "Start browser sign-in to begin.";
    controls();
  };
  const perform = async operation => {
    if (disposed || busy || (operation === "start" ? active : !active)) return;
    busy = true;
    controls();
    try {
      const view = operation === "start"
        ? await magpie.auth.start(binding, { method: "browser" })
        : await magpie.auth[operation](binding, active.flow);
      if (disposed) {
        if (pending(view)) await magpie.auth.cancel(binding, view.flow).catch(() => {});
        return;
      }
      present(view);
    } catch {
      if (!disposed) status.textContent = "Magpie could not complete sign-in. Check the flow or cancel and try again.";
    } finally {
      busy = false;
      if (!disposed) controls();
    }
  };
  const start = () => perform("start");
  const poll = () => perform("finish");
  const stop = () => perform("cancel");
  signin.addEventListener("click", start);
  finish.addEventListener("click", poll);
  cancel.addEventListener("click", stop);
  present(null);
  return Object.freeze({
    async dispose() {
      if (disposed) return;
      disposed = true;
      signin.removeEventListener("click", start);
      finish.removeEventListener("click", poll);
      cancel.removeEventListener("click", stop);
      address.dispose();
      signin.disabled = finish.disabled = cancel.disabled = true;
      if (active) await magpie.auth.cancel(binding, active.flow).catch(() => {});
      active = null;
    },
  });
}
