import { mountAuthentication } from "./auth.js";

try {
  await window.magpie.ready;
  const authentication = mountAuthentication(document, window.magpie);
  window.addEventListener("pagehide", () => { void authentication.dispose(); }, { once: true });
} catch {
  document.getElementById("status").textContent = "Open this page again from Magpie to sign in.";
}
