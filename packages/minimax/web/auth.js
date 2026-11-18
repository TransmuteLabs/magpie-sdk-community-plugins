import { createAuthLinkPresentation } from "./sdk-ui.js";

const bindings = Object.freeze({
  "minimax-code": "page.auth.minimax-code",
  "minimax-code-global": "page.auth.minimax-code-global",
});

export function mountAuthentication(document, magpie) {
  const element = id => document.getElementById(id);
  const region = element("region");
  const signin = element("signin");
  const finish = element("finish");
  const cancel = element("cancel");
  const status = element("status");
  const code = element("user-code");
  const address = createAuthLinkPresentation({
    container: element("auth-address"), input: element("auth-url"), select: element("auth-select"),
  });
  let active = null;
  let binding = "";
  let busy = false;
  let disposed = false;
  const pending = view => Boolean(view && ["waiting", "pending"].includes(view.state));
  const controls = () => {
    region.disabled = busy || pending(active);
    signin.disabled = busy || pending(active) || !Object.hasOwn(bindings, region.value);
    finish.disabled = cancel.disabled = busy || !pending(active);
  };
  const present = view => {
    active = pending(view) ? view : null;
    address.update(view);
    code.textContent = view?.userCode ? `Device code: ${view.userCode}` : "";
    status.textContent = view?.state === "complete" ? `Signed in: ${view.account.id}`
      : pending(view) ? "Approve the device code in your browser, then check sign-in."
      : view ? `Sign-in ${view.state}.` : "Choose a region to begin.";
    controls();
  };
  const perform = async operation => {
    if (disposed || busy) return;
    if (operation === "start") {
      if (active || !Object.hasOwn(bindings, region.value)) return;
      binding = bindings[region.value];
    } else if (!active) return;
    busy = true;
    controls();
    try {
      const view = operation === "start"
        ? await magpie.auth.start(binding, { method: "device" })
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
  const changed = () => { if (!disposed) controls(); };
  signin.addEventListener("click", start);
  finish.addEventListener("click", poll);
  cancel.addEventListener("click", stop);
  region.addEventListener("change", changed);
  present(null);
  return Object.freeze({
    async dispose() {
      if (disposed) return;
      disposed = true;
      signin.removeEventListener("click", start);
      finish.removeEventListener("click", poll);
      cancel.removeEventListener("click", stop);
      region.removeEventListener("change", changed);
      address.dispose();
      code.textContent = "";
      signin.disabled = finish.disabled = cancel.disabled = region.disabled = true;
      if (active) await magpie.auth.cancel(binding, active.flow).catch(() => {});
      active = null;
    },
  });
}
