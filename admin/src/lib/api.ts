const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "";

export class UnauthorizedError extends Error {
  constructor(message = "Sessao expirada. Entre novamente.") {
    super(message);
    this.name = "UnauthorizedError";
  }
}

export function isUnauthorizedError(err: unknown): err is UnauthorizedError {
  return err instanceof UnauthorizedError;
}

export type Collaborator = {
  id?: string;
  name: string;
  email: string;
  whatsapp: string;
  role: "director" | "supervisor" | "seller";
  rdstation_id: string;
  supervisor_id: string;
  active: boolean;
};

export type Alert = {
  id?: string;
  name: string;
  deal_stage_id: string;
  deal_stage_name: string;
  time_threshold_hours: number;
  repeat_interval_hours: number;
  message_template: string;
  recipient_ids: string[];
  active: boolean;
};

export type AllowlistEntry = {
  id?: string;
  phone_number: string;
  label: string;
  role: "director" | "supervisor" | "seller";
  collaborator_id: string;
  active: boolean;
  sync_pending?: boolean;
};

export type WhatsAppStatus = {
  instance: string;
  state: string;
};

export type WhatsAppQRCode = {
  instance: string;
  code?: string;
  base64?: string;
  pairing_code?: string;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });
  if (res.status === 401 && path !== "/admin/login") {
    if (path !== "/admin/logout") {
      await fetch(`${API_URL}/admin/logout`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
      }).catch(() => undefined);
    }
    throw new UnauthorizedError();
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `HTTP ${res.status}`);
  }
  return res.json() as Promise<T>;
}

export function login(email: string, password: string) {
  return request<{ email: string; must_change_password: boolean }>("/admin/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function logout() {
  return request<{ ok: boolean }>("/admin/logout", { method: "POST" });
}

export function changePassword(current_password: string, new_password: string) {
  return request<{ ok: boolean }>("/admin/change-password", {
    method: "POST",
    body: JSON.stringify({ current_password, new_password }),
  });
}

export function me() {
  return request<{ email: string; must_change_password: boolean }>("/admin/me");
}

export function listCollaborators(activeOnly = false) {
  return request<{ items: Collaborator[] }>(`/admin/collaborators${activeOnly ? "?active=true" : ""}`);
}

export function saveCollaborator(item: Collaborator) {
  const path = item.id ? `/admin/collaborators/${item.id}` : "/admin/collaborators";
  return request<Collaborator>(path, { method: item.id ? "PUT" : "POST", body: JSON.stringify(item) });
}

export function listAlerts() {
  return request<{ items: Alert[] }>("/admin/alerts");
}

export function saveAlert(item: Alert) {
  const path = item.id ? `/admin/alerts/${item.id}` : "/admin/alerts";
  return request<Alert>(path, { method: item.id ? "PUT" : "POST", body: JSON.stringify(item) });
}

export function testAlert(id: string) {
  return request<{
    ok: boolean;
    result: {
      alert_id: string;
      deals_matched: number;
      messages_sent: number;
      skipped_dedup: number;
      send_errors: number;
      forced: boolean;
      recipients: Collaborator[];
      matched_deal_ids: string[];
    };
  }>(`/admin/alerts/${id}/test`, { method: "POST" });
}

export function listAllowlist() {
  return request<{ items: AllowlistEntry[] }>("/admin/allowlist");
}

export function saveAllowlist(item: AllowlistEntry) {
  const path = item.id ? `/admin/allowlist/${item.id}` : "/admin/allowlist";
  return request<AllowlistEntry>(path, { method: item.id ? "PUT" : "POST", body: JSON.stringify(item) });
}

export function getWhatsAppStatus() {
  return request<WhatsAppStatus>("/admin/whatsapp/status");
}

export function generateWhatsAppQRCode() {
  return request<WhatsAppQRCode>("/admin/whatsapp/qrcode", { method: "POST" });
}

export function listStages() {
  return request<{ items: Array<{ ID?: string; Name?: string; id?: string; name?: string }> }>("/admin/rd-station/stages");
}
