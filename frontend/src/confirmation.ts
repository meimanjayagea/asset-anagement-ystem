import { shallowRef } from "vue";

export type ConfirmationOptions = {
  title: string;
  message: string;
  confirmLabel: string;
  subject?: string;
  tone?: "warning" | "danger" | "positive";
  icon?: "archive" | "restore" | "database" | "check" | "cancel";
  reasonLabel?: string;
};
export type ConfirmationResult = { reason: string };
export const confirmation = shallowRef<ConfirmationOptions | null>(null);
let resolvePending: ((value: ConfirmationResult | null) => void) | null = null;

export function confirmAction(options: ConfirmationOptions): Promise<ConfirmationResult | null> {
  if (resolvePending) return Promise.resolve(null);
  return new Promise(resolve => {
    resolvePending = resolve;
    confirmation.value = options;
  });
}

export function settleConfirmation(value: ConfirmationResult | null) {
  const resolve = resolvePending;
  resolvePending = null;
  confirmation.value = null;
  resolve?.(value);
}
