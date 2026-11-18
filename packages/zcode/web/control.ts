import type { AccountSelection, AccountSummary, ModuleUI, UIValue } from "@magpie/extension-sdk/ui";

export const bindings = {
  executionList: "page.execution-list",
  billingStatus: "page.billing-status",
  resetStatus: "page.reset-status",
  resetUse: "page.reset-use",
  resetSummary: "page.reset-summary",
  resetReconcile: "page.reset-reconcile",
  resetOpportunity: "page.reset-opportunity",
  resetOpportunityResolve: "page.reset-opportunity-resolve",
  resetResolve: "page.reset-resolve",
  resetAcknowledge: "page.reset-ack",
  billingClaim: "page.billing-claim",
  billingOffers: "page.billing-offers",
  billingCheckin: "page.billing-checkin",
  nativeRun: "page.native-run",
  nativeStop: "page.native-stop",
} as const;

type ReadAction = "executionList" | "billingStatus" | "resetStatus" | "resetSummary" | "billingOffers";
type ResetAction = "opportunity" | "reconcile" | "ack" | "resolve" | "opportunity.resolve";

export interface NativeInput {
  sessionId: string;
  commandId: string;
  model: string;
  text: string;
}

export interface ControlSnapshot {
  accounts: AccountSummary[];
  selected?: AccountSelection;
  busy: boolean;
  last?: UIValue;
}

export class ZcodeControl {
  #accounts: AccountSummary[] = [];
  #selected?: AccountSelection;
  #busy = false;
  #epoch = 0;
  #loading = 0;
  #reading = 0;
  #last?: UIValue;
  #native?: { sessionId: string; commandId: string; account: AccountSelection; stopped: boolean };

  constructor(private readonly backend: ModuleUI["backend"]) {}

  #invalidate(): void {
    this.#accounts = [];
    this.#selected = undefined;
    this.#last = undefined;
    this.#epoch++;
    this.#reading++;
  }

  snapshot(): ControlSnapshot {
    return {
      accounts: this.#accounts.map((account) => ({ ...account })),
      selected: this.#selected ? { ...this.#selected } : undefined,
      busy: this.#busy,
      last: this.#last === undefined ? undefined : structuredClone(this.#last),
    };
  }

  async refreshAccounts(): Promise<ControlSnapshot> {
    const loading = ++this.#loading;
    let values: AccountSummary[];
    try {
      values = await this.backend.accounts(bindings.executionList);
    } catch (error) {
      if (loading === this.#loading) this.#invalidate();
      throw error;
    }
    if (loading !== this.#loading) return this.snapshot();
    if (!Array.isArray(values) || values.length > 1024) {
      this.#invalidate();
      throw new Error("Host returned an invalid account list");
    }
    const seen = new Set<string>();
    const accounts = values.map((value): AccountSummary => {
      if (!value || typeof value.id !== "string" || !value.id ||
          new TextEncoder().encode(value.id).byteLength > 256 || /[\u0000-\u001f\u007f]/u.test(value.id) ||
          typeof value.generation !== "string" || !/^[1-9][0-9]{0,19}$/u.test(value.generation) ||
          BigInt(value.generation) > 18446744073709551615n || seen.has(value.id) ||
          value.expiresAt !== undefined && (!Number.isSafeInteger(value.expiresAt) || value.expiresAt < 0)) {
        this.#invalidate();
        throw new Error("Host returned an invalid account summary");
      }
      seen.add(value.id);
      return value.expiresAt === undefined
        ? { id: value.id, generation: value.generation }
        : { id: value.id, generation: value.generation, expiresAt: value.expiresAt };
    });
    this.#accounts = accounts;
    const selected = this.#selected;
    if (selected && !accounts.some((account) =>
      account.id === selected.id && account.generation === selected.expectedGeneration)) {
      this.#selected = undefined;
      this.#last = undefined;
      this.#epoch++;
    }
    return this.snapshot();
  }

  select(id: string): ControlSnapshot {
    if (this.#busy) throw new Error("Wait for the current write to finish");
    const account = this.#accounts.find((value) => value.id === id);
    if (!account) throw new Error("Select an account returned by the host");
    this.#selected = { id: account.id, expectedGeneration: account.generation };
    this.#last = undefined;
    this.#epoch++;
    return this.snapshot();
  }

  #selection(): AccountSelection {
    if (!this.#selected) throw new Error("Select a current account first");
    return { ...this.#selected };
  }

  async read(action: ReadAction): Promise<UIValue> {
    if (this.#busy) throw new Error("Wait for the current write to finish");
    if (action !== "executionList" && action !== "billingStatus" && action !== "resetStatus" &&
        action !== "resetSummary" && action !== "billingOffers") {
      throw new Error("Choose a declared read action");
    }
    const account = this.#selection();
    const epoch = this.#epoch;
    const reading = ++this.#reading;
    let value: UIValue;
    try {
      value = await this.backend.call(bindings[action], {}, { account });
    } catch (error) {
      if (epoch === this.#epoch) this.#invalidate();
      throw error;
    }
    if (epoch !== this.#epoch || reading !== this.#reading) {
      throw new Error("Account selection changed while reading");
    }
    this.#last = structuredClone(value);
    return structuredClone(value);
  }

  async #write(binding: string, input: UIValue): Promise<UIValue> {
    if (this.#busy) throw new Error("Wait for the current write to finish");
    const account = this.#selection();
    const epoch = this.#epoch;
    this.#busy = true;
    this.#reading++;
    this.#last = undefined;
    try {
      const value = await this.backend.call(binding, structuredClone(input), { account });
      if (epoch !== this.#epoch) throw new Error("Account selection changed while writing");
      this.#last = structuredClone(value);
      return structuredClone(value);
    } catch (error) {
      if (epoch === this.#epoch) this.#invalidate();
      throw error;
    } finally {
      this.#busy = false;
    }
  }

  reset(window: "FIVE_HOUR" | "WEEK"): Promise<UIValue> {
    if (window !== "FIVE_HOUR" && window !== "WEEK") {
      return Promise.reject(new Error("Choose a supported reset window"));
    }
    return this.#write(bindings.resetUse, { window });
  }

  resetControl(action: ResetAction): Promise<UIValue> {
    switch (action) {
      case "opportunity": return this.#write(bindings.resetOpportunity, {});
      case "reconcile": return this.#write(bindings.resetReconcile, {});
      case "ack": return this.#write(bindings.resetAcknowledge, {});
      case "resolve": return this.#write(bindings.resetResolve, { acknowledgment: "vendor-may-have-spent-reset" });
      case "opportunity.resolve": return this.#write(bindings.resetOpportunityResolve, { acknowledgment: "vendor-may-have-granted-reset" });
      default: return Promise.reject(new Error("Choose a declared reset control"));
    }
  }

  claim(planId: string): Promise<UIValue> {
    if (!planId || planId.trim() !== planId || planId.length > 128 || /[\u0000-\u001f\u007f]/u.test(planId)) {
      return Promise.reject(new Error("Enter one exact plan ID"));
    }
    return this.#write(bindings.billingClaim, { planId });
  }

  checkin(): Promise<UIValue> {
    return this.#write(bindings.billingCheckin, {});
  }

  async nativeRun(input: NativeInput): Promise<UIValue> {
    if (this.#busy) throw new Error("Wait for the current write to finish");
    const token = (value: string, limit: number) => typeof value === "string" && value !== "" &&
      value.trim() === value && new TextEncoder().encode(value).length <= limit &&
      !/[\u0000-\u001f\u007f-\u009f]/u.test(value);
    if (!token(input.sessionId, 256) || input.sessionId.startsWith("-") ||
        !token(input.commandId, 256) || !token(input.model, 1024) ||
        typeof input.text !== "string" || !input.text.trim() || new TextEncoder().encode(input.text).length > 65536) {
      throw new Error("Enter a bounded native session, command ID, gateway model and text");
    }
    const selected = this.#selection();
    this.#native = { sessionId: input.sessionId, commandId: input.commandId, account: selected, stopped: false };
    try {
      return await this.#write(bindings.nativeRun, { ...input });
    } finally {
      this.#native = undefined;
    }
  }

  async nativeStop(): Promise<UIValue> {
    const active = this.#native;
    if (!active || active.stopped) throw new Error("No matching foreground command can be stopped");
    active.stopped = true;
    return this.backend.call(bindings.nativeStop,
      { sessionId: active.sessionId, commandId: active.commandId }, { account: { ...active.account } });
  }
}
