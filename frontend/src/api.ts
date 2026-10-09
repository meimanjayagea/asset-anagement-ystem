export interface User {
  id: number;
  org_id: number;
  name: string;
  email: string;
  role: string;
  all_branches: boolean;
  branch_ids: number[];
}
export interface Asset {
  id: number;
  tag: string;
  name: string;
  serial_number: string;
  status: string;
  category_name: string;
  category_id: number;
  location_name: string;
  location_id: number;
  custodian: string;
  purchase_date: string;
  purchase_cost: number;
  salvage_value: number;
  book_value: number;
  useful_life_months: number;
  warranty_until: string | null;
  version: number;
  branch_id: number;
  branch_name: string;
  next_maintenance_date: string | null;
}
export async function api<T = any>(path: string, body?: unknown): Promise<T> {
  const response = await fetch("/api" + path, {
    method: body === undefined ? "GET" : "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-Requested-With": "AssetFlow",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok) {
    if (response.status === 401 && path !== "/login")
      window.dispatchEvent(new Event("session-expired"));
    throw new Error(data.error || "Request gagal");
  }
  return data;
}
export const money = (v: number) =>
  new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 0,
  }).format(v || 0);
export const date = (v: string) =>
  v
    ? new Intl.DateTimeFormat("id-ID", { dateStyle: "medium" }).format(
        new Date(v),
      )
    : "—";

export const timestamp = (v: string) =>
  v
    ? new Intl.DateTimeFormat("id-ID", {
        dateStyle: "medium",
        timeStyle: "medium",
        timeZone: "Asia/Jakarta",
      }).format(new Date(v)) + " WIB"
    : "—";
