import type { ModuleUI, AccountSummary, WindowAuthFlowView } from "@magpie/extension-sdk/ui";
import { createAuthLinkPresentation } from "./sdk-ui.js";

declare const magpie: ModuleUI;
const output = document.querySelector<HTMLPreElement>("#output")!;
const picker = document.querySelector<HTMLSelectElement>("#account")!;
const key = document.querySelector<HTMLInputElement>("#key")!;
const authLink = createAuthLinkPresentation({
  container: document.querySelector<HTMLElement>("#auth-address")!,
  input: document.querySelector<HTMLInputElement>("#auth-url")!,
  select: document.querySelector<HTMLButtonElement>("#auth-select")!,
});
const finish = document.querySelector<HTMLButtonElement>("#finish")!;
const cancel = document.querySelector<HTMLButtonElement>("#cancel")!;
const zdr = document.querySelector<HTMLElement>("#zdr")!;
let accounts: AccountSummary[] = [];
let flow: WindowAuthFlowView | null = null;

function report(error: unknown): void {
  output.textContent = error instanceof Error ? error.message : String(error);
}

function renderFlow(value: WindowAuthFlowView | null): void {
  flow = value;
  const pending = value !== null && (value.state === "waiting" || value.state === "pending");
  finish.hidden = cancel.hidden = !pending;
  authLink.update(value);
  output.textContent = value ? `Sign-in: ${value.state}` : "Sign-in canceled.";
}

async function refresh(): Promise<void> {
  await magpie.ready;
  accounts = await magpie.backend.accounts("page.status");
  picker.replaceChildren(new Option("Choose an account", ""), ...accounts.map(account => new Option(account.id, account.id)));
  const config = await magpie.config.get();
  zdr.textContent = `Zero Data Retention: ${config.values.zdr === true ? "On" : "Off"}. Change it in Providers → Command Code, where enabling it requires confirmation.`;
  output.textContent = "Choose an account to read its plan and balances. Jev is available through SystemOne decisions when your plan permits.";
}

async function signIn(method: string): Promise<void> {
  const secret = key.value;
  key.value = "";
  renderFlow(await magpie.auth.start("page.auth", { method, ...(method === "api-key" ? { input: { apiKey: secret } } : {}) }));
}

document.querySelector("#refresh")!.addEventListener("click", () => void refresh().catch(report));
document.querySelector("#key-signin")!.addEventListener("click", () => void signIn("api-key").catch(report));
document.querySelector("#browser")!.addEventListener("click", () => void signIn("browser").catch(report));
document.querySelector("#import")!.addEventListener("click", () => void signIn("cli-import").catch(report));
finish.addEventListener("click", () => void (async () => {
  if (!flow) return;
  const result = await magpie.auth.finish("page.auth", flow.flow);
  renderFlow(result);
  if (result.state === "complete") await refresh();
})().catch(report));
cancel.addEventListener("click", () => void (async () => {
  if (!flow) return;
  renderFlow(await magpie.auth.cancel("page.auth", flow.flow));
})().catch(report));
document.querySelector("#status")!.addEventListener("click", () => void (async () => {
  const account = accounts.find(account => account.id === picker.value);
  if (!account) throw new Error("Choose an account first.");
  const value = await magpie.backend.call("page.status", {}, { account: { id: account.id, expectedGeneration: account.generation } });
  output.textContent = JSON.stringify(value, null, 2);
})().catch(report));
void refresh().catch(report);
window.addEventListener("pagehide", () => authLink.dispose(), { once: true });
