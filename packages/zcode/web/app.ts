import { ui } from "./sdk-ui.js";
import { mountControlPage } from "./page.js";

async function start(): Promise<void> {
  try {
    await ui.ready;
    ui.onTheme((theme) => { document.documentElement.dataset.theme = theme.dark ? "dark" : "light"; });
    if (ui.theme) document.documentElement.dataset.theme = ui.theme.dark ? "dark" : "light";
    const dialog = document.getElementById("confirm-dialog") as HTMLDialogElement;
    const confirmation = document.getElementById("confirm-text")!;
    document.getElementById("confirm-cancel")!.addEventListener("click", () => dialog.close("cancel"));
    document.getElementById("confirm-accept")!.addEventListener("click", () => dialog.close("confirm"));
    const page = mountControlPage(document, ui.backend, (message) => new Promise<boolean>((resolve) => {
      confirmation.textContent = message;
      dialog.returnValue = "";
      dialog.addEventListener("close", () => resolve(dialog.returnValue === "confirm"), { once: true });
      dialog.showModal();
    }));
    window.addEventListener("pagehide", () => page.dispose(), { once: true });
    await page.ready;
  } catch {
    document.getElementById("status")!.textContent = "Связь с Magpie недоступна. Откройте страницу заново.";
  }
}

void start();
