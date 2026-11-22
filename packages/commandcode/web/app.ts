import type { ModuleUI, AccountSummary } from "@magpie/extension-sdk/ui";

declare const magpie: ModuleUI;
const output = document.querySelector<HTMLPreElement>("#output")!;
const picker = document.querySelector<HTMLSelectElement>("#account")!;
const status = document.querySelector<HTMLButtonElement>("#status")!;
let accounts: AccountSummary[] = [];

function report(error: unknown): void {
  output.textContent = error instanceof Error ? error.message : String(error);
}

function showSelection(): void {
  status.disabled = !picker.value;
  output.textContent = accounts.length ? "Choose an account to read its plan and balances." : "Sign in through Providers → Add → Subscriptions.";
}

async function refresh(): Promise<void> {
  await magpie.ready;
  const before = picker.value;
  accounts = await magpie.backend.accounts("account.status");
  picker.replaceChildren(new Option("Choose an account", ""), ...accounts.map(account => new Option(account.name || account.id, account.id)));
  picker.value = accounts.find(account => account.id === before)?.id ?? (accounts.length === 1 ? accounts[0].id : "");
  showSelection();
}

async function readStatus(): Promise<void> {
  const account = accounts.find(account => account.id === picker.value);
  if (!account) return;
  const isSelected = () => picker.value === account.id && accounts.includes(account);
  try {
    const value = await magpie.backend.call("account.status", {}, { account: { id: account.id, expectedGeneration: account.generation } });
    if (isSelected()) output.textContent = JSON.stringify(value, null, 2);
  } catch (error) {
    if (isSelected()) report(error);
  }
}

picker.addEventListener("change", showSelection);
document.querySelector("#refresh")!.addEventListener("click", () => void refresh().catch(report));
status.addEventListener("click", () => void readStatus());
void refresh().catch(report);
