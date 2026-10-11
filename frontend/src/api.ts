export interface User {
  id: number;
  org_id: number;
  organization_name: string;
  name: string;
  email: string;
  employee_id: string;
  role: string;
  all_branches: boolean;
  branch_ids: number[];
  active_branch_id: number;
  capabilities: string[];
}
export interface RoleOption {
  id: string;
  label: string;
}
export interface LoginOrganization {
  id: number;
  code: string;
  name: string;
}
export interface Asset {
  cover_photo_id?: number | null;
  brand: string;
  model: string;
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
  supplier_name: string;
  acquisition_reference: string;
  purchase_cost: number;
  salvage_value: number;
  book_value: number;
  useful_life_months: number;
  warranty_until: string | null;
  depreciation_method: string;
  depreciation_start_date: string;
  version: number;
  branch_id: number;
  branch_name: string;
  next_maintenance_date: string | null;
  deleted_at?: string | null;
}
export class ApiError extends Error {
  constructor(message: string, public status: number) {
    super(message);
  }
}
export async function api<T = any>(
  path: string,
  body?: unknown,
  method?: "GET" | "POST" | "PUT" | "DELETE",
): Promise<T> {
  const response = await fetch("/api" + path, {
    method: method || (body === undefined ? "GET" : "POST"),
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-Requested-With": "AssetFlow",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json().catch(() => {
    throw new ApiError("Layanan tidak tersedia. Coba lagi nanti.", response.status);
  });
  if (!response.ok) {
    if (response.status === 401 && path !== "/login")
      window.dispatchEvent(new Event("session-expired"));
    throw new ApiError(data.error || "Request gagal", response.status);
  }
  return data;
}
export const money = (v: number) =>
  new Intl.NumberFormat(locale.value === "id" ? "id-ID" : "en-US", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 0,
  }).format(v || 0);
export const date = (v: string) =>
  v
    ? new Intl.DateTimeFormat(locale.value === "id" ? "id-ID" : "en-US", { dateStyle: "medium", timeZone: "Asia/Jakarta" }).format(
        new Date(v.length === 10 ? v + "T12:00:00+07:00" : v),
      )
    : "—";

export const timestamp = (v: string) =>
  v
    ? new Intl.DateTimeFormat(locale.value === "id" ? "id-ID" : "en-US", {
        dateStyle: "medium",
        timeStyle: "medium",
        timeZone: "Asia/Jakarta",
      }).format(new Date(v)) + " WIB"
    : "—";
import { locale } from "./preferences";
