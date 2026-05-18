"use client";

import { useEffect, useMemo, useState } from "react";
import {
  Bell,
  Check,
  Edit3,
  ListChecks,
  LogOut,
  Plus,
  QrCode,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Users,
  X,
} from "lucide-react";
import QRCode from "qrcode";
import {
  Alert,
  AllowlistEntry,
  Collaborator,
  generateWhatsAppQRCode,
  getWhatsAppStatus,
  listAlerts,
  listAllowlist,
  listCollaborators,
  listStages,
  logout,
  me,
  saveAlert,
  saveAllowlist,
  saveCollaborator,
  testAlert,
  WhatsAppQRCode,
  WhatsAppStatus,
  isUnauthorizedError,
} from "@/lib/api";

type Tab = "overview" | "whatsapp" | "collaborators" | "alerts" | "allowlist";

const emptyCollaborator: Collaborator = { name: "", email: "", whatsapp: "", role: "seller", rdstation_id: "", supervisor_id: "", active: true };
const emptyAllowlist: AllowlistEntry = { phone_number: "", label: "", role: "seller", collaborator_id: "", active: true };
const emptyAlert: Alert = {
  name: "",
  deal_stage_id: "",
  deal_stage_name: "",
  time_threshold_hours: 48,
  repeat_interval_hours: 48,
  message_template: "A negociacao {{deal_name}} esta parada em {{deal_stage}} ha {{days_in_stage}} dias.",
  recipient_ids: [],
  active: false,
};

export default function DashboardPage() {
  const [tab, setTab] = useState<Tab>("overview");
  const [email, setEmail] = useState("");
  const [collaborators, setCollaborators] = useState<Collaborator[]>([]);
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [allowlist, setAllowlist] = useState<AllowlistEntry[]>([]);
  const [whatsappStatus, setWhatsAppStatus] = useState<WhatsAppStatus | null>(null);
  const [whatsappQR, setWhatsAppQR] = useState<WhatsAppQRCode | null>(null);
  const [qrImage, setQrImage] = useState("");
  const [stages, setStages] = useState<Array<{ id: string; name: string }>>([]);
  const [collaboratorDraft, setCollaboratorDraft] = useState<Collaborator>(emptyCollaborator);
  const [allowlistDraft, setAllowlistDraft] = useState<AllowlistEntry>(emptyAllowlist);
  const [alertDraft, setAlertDraft] = useState<Alert>(emptyAlert);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(true);
  const [testingAlertID, setTestingAlertID] = useState<string | null>(null);
  const [connectingWhatsApp, setConnectingWhatsApp] = useState(false);

  async function load() {
    setLoading(true);
    setMessage("");
    try {
      const [profile, c, a, w, ws] = await Promise.all([me(), listCollaborators(), listAlerts(), listAllowlist(), getWhatsAppStatus().catch(() => null)]);
      if (profile.must_change_password) {
        window.location.href = "/change-password";
        return;
      }
      setEmail(profile.email);
      setCollaborators((c.items ?? []).map((item) => ({ ...item, role: item.role || "seller", rdstation_id: item.rdstation_id || "", supervisor_id: item.supervisor_id || "" })));
      setAlerts((a.items ?? []).map((item) => ({ ...item, repeat_interval_hours: item.repeat_interval_hours || 48 })));
      setAllowlist((w.items ?? []).map((item) => ({ ...item, role: item.role || "seller", collaborator_id: item.collaborator_id || "" })));
      setWhatsAppStatus(ws);
      listStages()
        .then((result) => {
          setStages((result.items ?? []).map((s) => ({ id: s.id ?? s.ID ?? "", name: s.name ?? s.Name ?? "" })).filter((s) => s.id));
        })
        .catch(() => setStages([]));
    } catch (err) {
      if (isUnauthorizedError(err)) {
        window.location.replace("/login");
        return;
      }
      setMessage(err instanceof Error ? err.message : "Falha ao carregar painel");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  const activeCollaborators = useMemo(() => collaborators.filter((item) => item.active), [collaborators]);
  const activeAlerts = useMemo(() => alerts.filter((item) => item.active), [alerts]);
  const activeAllowlist = useMemo(() => allowlist.filter((item) => item.active), [allowlist]);
  const alertRecipientOptions = useMemo(
    () => collaborators.filter((item) => item.active || alertDraft.recipient_ids.includes(item.id ?? "")),
    [collaborators, alertDraft.recipient_ids],
  );

  async function submitCollaborator(e: React.FormEvent) {
    e.preventDefault();
    await saveCollaborator(collaboratorDraft);
    setCollaboratorDraft(emptyCollaborator);
    await load();
  }

  async function submitAllowlist(e: React.FormEvent) {
    e.preventDefault();
    await saveAllowlist(allowlistDraft);
    setAllowlistDraft(emptyAllowlist);
    await load();
  }

  async function submitAlert(e: React.FormEvent) {
    e.preventDefault();
    const stage = stages.find((item) => item.id === alertDraft.deal_stage_id);
    setMessage("");
    try {
      await saveAlert({ ...alertDraft, deal_stage_name: stage?.name || alertDraft.deal_stage_name });
      setAlertDraft(emptyAlert);
      await load();
    } catch (err) {
      const detail = err instanceof Error ? err.message : "";
      setMessage(detail === "invalid_input" ? "Nao foi possivel salvar: confira estagio, frequencia e selecione ao menos um colaborador ativo se o alerta estiver ativo." : detail || "Falha ao salvar alerta");
    }
  }

  async function toggleCollaborator(item: Collaborator) {
    await saveCollaborator({ ...item, active: !item.active });
    await load();
  }

  async function toggleAllowlist(item: AllowlistEntry) {
    await saveAllowlist({ ...item, active: !item.active });
    await load();
  }

  async function toggleAlert(item: Alert) {
    setMessage("");
    try {
      await saveAlert({ ...item, active: !item.active, repeat_interval_hours: item.repeat_interval_hours || 48 });
      await load();
    } catch (err) {
      const detail = err instanceof Error ? err.message : "";
      setMessage(detail === "invalid_input" ? "Nao foi possivel ativar: este alerta precisa ter pelo menos um destinatario ativo." : detail || "Falha ao atualizar alerta");
    }
  }

  async function runAlertDevOnly(item: Alert) {
    if (!item.id || testingAlertID) return;
    setTestingAlertID(item.id);
    setMessage("");
    try {
      const result = await testAlert(item.id);
      const names = result.result.recipients.map((recipient) => `${recipient.name} (${recipient.whatsapp})`).join(", ");
      if (result.result.messages_sent > 0) {
        setMessage(`Alerta executado agora: ${result.result.deals_matched} negociacoes elegiveis, ${result.result.messages_sent} mensagens enviadas. Destinatarios: ${names}`);
      } else if (result.result.deals_matched === 0) {
        setMessage("Alerta executado agora, mas nao havia negociacoes elegiveis para esse estagio e tempo configurados.");
      } else if (result.result.send_errors > 0) {
        setMessage(`Alerta encontrou ${result.result.deals_matched} negociacoes, mas falhou ao enviar para ${result.result.send_errors} destinatario(s).`);
      } else {
        setMessage(`Alerta executado agora, mas nenhuma mensagem foi enviada. Destinatarios ativos encontrados: ${names || "nenhum"}.`);
      }
    } catch (err) {
      setMessage(err instanceof Error ? err.message : "Falha ao testar alerta");
    } finally {
      setTestingAlertID(null);
    }
  }

  async function refreshWhatsAppStatus() {
    setMessage("");
    try {
      setWhatsAppStatus(await getWhatsAppStatus());
    } catch (err) {
      setMessage(err instanceof Error ? err.message : "Falha ao consultar status do WhatsApp");
    }
  }

  async function generateQRCode() {
    setConnectingWhatsApp(true);
    setMessage("");
    setWhatsAppQR(null);
    setQrImage("");
    try {
      const qr = await generateWhatsAppQRCode();
      setWhatsAppQR(qr);
      const image = normalizeQRImage(qr.base64);
      setQrImage(image || (qr.code ? await QRCode.toDataURL(qr.code, { width: 280, margin: 1 }) : ""));
      await refreshWhatsAppStatus();
    } catch (err) {
      setMessage(err instanceof Error ? err.message : "Falha ao gerar QR Code do WhatsApp");
    } finally {
      setConnectingWhatsApp(false);
    }
  }

  function editAlert(item: Alert) {
    setAlertDraft({ ...item, repeat_interval_hours: item.repeat_interval_hours || 48, recipient_ids: item.recipient_ids ?? [] });
    setTab("alerts");
  }

function recipientSummary(ids: string[]) {
    const selected = ids
      .map((id) => collaborators.find((item) => item.id === id))
      .filter(Boolean) as Collaborator[];
    if (selected.length === 0) return "Sem destinatarios";
    return selected.map((item) => `${item.name} (${item.whatsapp})`).join(", ");
  }

  function roleLabel(role: string) {
    if (role === "director") return "Diretoria";
    if (role === "supervisor") return "Supervisor";
    return "Vendedor";
  }

  return (
    <main className="min-h-screen p-2 text-sm md:p-4">
      <div className="mx-auto grid max-w-7xl gap-3 lg:grid-cols-[13.5rem_1fr]">
        <aside className="glass rounded-md p-3 lg:sticky lg:top-4 lg:h-[calc(100vh-2rem)]">
          <div className="mb-3 flex items-center gap-2">
            <div className="grid h-9 w-9 shrink-0 place-items-center overflow-hidden rounded-md bg-white/92 p-1.5">
              <img src="/synova-logo.png" alt="Synova" className="max-h-full max-w-full object-contain" />
            </div>
            <div className="min-w-0">
              <h1 className="truncate font-semibold">Synova Admin</h1>
              <p className="text-xs text-emerald-50/60">{email || "Painel operacional"}</p>
            </div>
          </div>

          <nav className="grid grid-cols-2 gap-1.5 lg:grid-cols-1">
            <NavButton active={tab === "overview"} icon={<ShieldCheck size={17} />} onClick={() => setTab("overview")} label="Resumo" />
            <NavButton active={tab === "whatsapp"} icon={<Smartphone size={17} />} onClick={() => setTab("whatsapp")} label="WhatsApp" />
            <NavButton active={tab === "collaborators"} icon={<Users size={17} />} onClick={() => setTab("collaborators")} label="Colaboradores" />
            <NavButton active={tab === "alerts"} icon={<Bell size={17} />} onClick={() => setTab("alerts")} label="Alertas" />
            <NavButton active={tab === "allowlist"} icon={<ListChecks size={17} />} onClick={() => setTab("allowlist")} label="Allowlist do bot" />
          </nav>

          <div className="mt-3 grid grid-cols-2 gap-1.5 border-t border-white/10 pt-3 lg:grid-cols-1">
            <button onClick={load} className="flex items-center justify-center gap-2 rounded-md border border-white/10 px-2.5 py-2 text-xs hover:bg-white/7 lg:justify-start">
              <RefreshCw size={16} /> Atualizar
            </button>
            <button
              onClick={async () => {
                await logout().catch(() => undefined);
                window.location.replace("/login");
              }}
              className="flex items-center justify-center gap-2 rounded-md border border-white/10 px-2.5 py-2 text-xs hover:bg-white/7 lg:justify-start"
            >
              <LogOut size={16} /> Sair
            </button>
          </div>
        </aside>

        <section className="grid content-start gap-3">
          {message && <p className="rounded-md border border-amber-200/25 bg-amber-300/10 px-3 py-2 text-xs text-amber-50">{message}</p>}
          {loading && <p className="glass rounded-md p-4 text-sm">Carregando painel...</p>}

          {tab === "overview" && (
            <>
              <Header title="Resumo" subtitle="Estado atual do painel, alertas e acesso ao bot." />
              <div className="grid gap-3 sm:grid-cols-3">
                <Metric title="Colaboradores ativos" value={activeCollaborators.length} icon={<Users />} />
                <Metric title="Alertas ativos" value={activeAlerts.length} icon={<Bell />} />
                <Metric title="Numeros na allowlist" value={activeAllowlist.length} icon={<ListChecks />} />
              </div>
              <Panel title="Como os envios funcionam">
                <div className="grid gap-3 text-sm text-emerald-50/72">
                  <p><strong className="text-orange-100">Colaboradores</strong> recebem alertas proativos.</p>
                  <p><strong className="text-orange-100">Allowlist</strong> controla quem pode conversar com o bot no WhatsApp.</p>
                  <p><strong className="text-orange-100">Frequencia</strong> fica em cada alerta: horas para primeira mensagem e intervalo para repetir para a mesma negociacao.</p>
                </div>
              </Panel>
            </>
          )}

          {tab === "whatsapp" && (
            <>
              <Header title="WhatsApp" subtitle="Conexao da instancia Evolution API usada pela Sil." />
              <div className="grid gap-3 xl:grid-cols-[22rem_1fr]">
                <Panel title="Status da conexao">
                  <div className="grid gap-3">
                    <div className="rounded-md border border-white/10 bg-black/10 p-3">
                      <p className="text-xs uppercase text-emerald-50/58">Instancia</p>
                      <p className="mt-1 font-medium">{whatsappStatus?.instance || "Nao informado"}</p>
                    </div>
                    <div className="rounded-md border border-white/10 bg-black/10 p-3">
                      <p className="text-xs uppercase text-emerald-50/58">Estado</p>
                      <p className={`mt-1 font-semibold ${isWhatsAppConnected(whatsappStatus?.state) ? "text-emerald-200" : "text-amber-100"}`}>
                        {statusLabel(whatsappStatus?.state)}
                      </p>
                    </div>
                    <button onClick={refreshWhatsAppStatus} className="flex items-center justify-center gap-2 rounded-md border border-white/10 px-3 py-2 text-sm hover:bg-white/7">
                      <RefreshCw size={16} /> Atualizar status
                    </button>
                    {!isWhatsAppConnected(whatsappStatus?.state) && (
                      <PrimaryButton onClick={generateQRCode} disabled={connectingWhatsApp}>
                        <QrCode size={16} /> {connectingWhatsApp ? "Gerando..." : "Gerar QR Code"}
                      </PrimaryButton>
                    )}
                  </div>
                </Panel>
                <Panel title="Conectar telefone">
                  <div className="grid min-h-72 place-items-center rounded-md border border-white/10 bg-black/10 p-4 text-center">
                    {isWhatsAppConnected(whatsappStatus?.state) ? (
                      <div className="max-w-md">
                        <div className="mx-auto mb-3 grid h-14 w-14 place-items-center rounded-md bg-emerald-300/15 text-emerald-100">
                          <Check size={28} />
                        </div>
                        <h3 className="font-semibold">WhatsApp conectado</h3>
                        <p className="mt-2 text-sm leading-relaxed text-emerald-50/68">A instancia esta pronta para receber e enviar mensagens pela Sil.</p>
                      </div>
                    ) : qrImage ? (
                      <div className="grid justify-items-center gap-3">
                        <img src={qrImage} alt="QR Code para conectar WhatsApp" className="h-72 w-72 rounded-md bg-white p-3" />
                        <p className="max-w-md text-sm leading-relaxed text-emerald-50/68">Escaneie este QR Code no WhatsApp do telefone que sera usado pela Sil.</p>
                        {whatsappQR?.pairing_code && <p className="rounded-md border border-white/10 px-3 py-2 text-sm">Codigo de pareamento: <strong>{whatsappQR.pairing_code}</strong></p>}
                      </div>
                    ) : (
                      <div className="max-w-md">
                        <QrCode className="mx-auto mb-3 text-emerald-100/80" size={48} />
                        <h3 className="font-semibold">Aguardando QR Code</h3>
                        <p className="mt-2 text-sm leading-relaxed text-emerald-50/68">Quando a instancia nao estiver conectada, gere um QR Code para vincular o WhatsApp pelo aplicativo no celular.</p>
                      </div>
                    )}
                  </div>
                </Panel>
              </div>
            </>
          )}

          {tab === "collaborators" && (
            <>
              <Header title="Colaboradores" subtitle="Pessoas que podem receber mensagens de alerta." />
              <div className="grid gap-3 xl:grid-cols-[22rem_1fr]">
                <Panel title="Cadastrar colaborador">
                  <form onSubmit={submitCollaborator} className="grid gap-3">
                    <Field label="Nome" help="Aparece na selecao de destinatarios dos alertas.">
                      <input className="field" value={collaboratorDraft.name} onChange={(e) => setCollaboratorDraft({ ...collaboratorDraft, name: e.target.value })} />
                    </Field>
                    <Field label="E-mail">
                      <input className="field" type="email" value={collaboratorDraft.email} onChange={(e) => setCollaboratorDraft({ ...collaboratorDraft, email: e.target.value })} />
                    </Field>
                    <Field label="WhatsApp para receber alertas" help="Formato E.164, exemplo: +5524999999999.">
                      <input className="field" value={collaboratorDraft.whatsapp} onChange={(e) => setCollaboratorDraft({ ...collaboratorDraft, whatsapp: e.target.value })} />
                    </Field>
                    <Field label="Perfil de acesso">
                      <select className="field" value={collaboratorDraft.role} onChange={(e) => setCollaboratorDraft({ ...collaboratorDraft, role: e.target.value as Collaborator["role"] })}>
                        <option value="director">Diretoria</option>
                        <option value="supervisor">Supervisor</option>
                        <option value="seller">Vendedor</option>
                      </select>
                    </Field>
                    <Field label="ID do usuario no RD Station" help="Usado para limitar negocios por responsavel.">
                      <input className="field" value={collaboratorDraft.rdstation_id} onChange={(e) => setCollaboratorDraft({ ...collaboratorDraft, rdstation_id: e.target.value })} />
                    </Field>
                    <Field label="Supervisor" help="Define equipe: vendedores vinculados ao supervisor aparecem para ele.">
                      <select className="field" value={collaboratorDraft.supervisor_id} onChange={(e) => setCollaboratorDraft({ ...collaboratorDraft, supervisor_id: e.target.value })}>
                        <option value="">Sem supervisor</option>
                        {collaborators.filter((item) => item.role === "supervisor").map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
                      </select>
                    </Field>
                    <PrimaryButton><Plus size={16} /> Salvar colaborador</PrimaryButton>
                  </form>
                </Panel>
                <Panel title="Lista de colaboradores">
                  <Rows items={collaborators} getKey={(item) => item.id ?? item.email} render={(item) => (
                    <Row title={item.name} subtitle={`${roleLabel(item.role)} - ${item.email} - ${item.whatsapp}${item.rdstation_id ? ` - RD ${item.rdstation_id}` : ""}`} active={item.active} onToggle={() => toggleCollaborator(item)} />
                  )} />
                </Panel>
              </div>
            </>
          )}

          {tab === "alerts" && (
            <>
              <Header title="Alertas" subtitle="Mensagens proativas sobre negociacoes paradas no RD Station." />
              <div className="grid gap-3 xl:grid-cols-[25rem_1fr]">
                <Panel title={alertDraft.id ? "Editar alerta" : "Configurar alerta"}>
                  <form onSubmit={submitAlert} className="grid gap-3">
                    <Field label="Nome interno do alerta">
                      <input className="field" value={alertDraft.name} onChange={(e) => setAlertDraft({ ...alertDraft, name: e.target.value })} />
                    </Field>
                    <Field label="Estagio monitorado no RD Station">
                      <select className="field" value={alertDraft.deal_stage_id} onChange={(e) => setAlertDraft({ ...alertDraft, deal_stage_id: e.target.value })}>
                        <option value="">Selecione um estagio</option>
                        {stages.map((stage) => <option key={stage.id} value={stage.id}>{stage.name}</option>)}
                      </select>
                    </Field>
                    <div className="grid gap-3 sm:grid-cols-2">
                      <Field label="Enviar apos" help="Primeiro disparo quando a negociacao ficar esse tempo no estagio.">
                        <div className="flex items-center gap-2">
                          <input className="field" type="number" min={1} value={alertDraft.time_threshold_hours} onChange={(e) => setAlertDraft({ ...alertDraft, time_threshold_hours: Number(e.target.value) })} />
                          <span className="text-sm text-emerald-50/68">horas</span>
                        </div>
                      </Field>
                      <Field label="Repetir a cada" help="Para a mesma negociacao. Use 48 para repetir a cada 2 dias.">
                        <div className="flex items-center gap-2">
                          <input className="field" type="number" min={1} value={alertDraft.repeat_interval_hours} onChange={(e) => setAlertDraft({ ...alertDraft, repeat_interval_hours: Number(e.target.value) })} />
                          <span className="text-sm text-emerald-50/68">horas</span>
                        </div>
                      </Field>
                    </div>
                    <Field label="Mensagem enviada">
                      <textarea className="field min-h-28" value={alertDraft.message_template} onChange={(e) => setAlertDraft({ ...alertDraft, message_template: e.target.value })} />
                    </Field>
                    <Field label="Quem recebe este alerta" help="Colaboradores inativos aparecem se ja estavam neste alerta, mas nao recebem envios.">
                      <div className="grid gap-2 rounded-md border border-white/10 p-2.5">
                        {alertRecipientOptions.length === 0 && <p className="text-sm text-emerald-50/62">Cadastre um colaborador ativo primeiro.</p>}
                        {alertRecipientOptions.map((item) => (
                          <label key={item.id} className="flex items-start gap-2 text-sm">
                            <input
                              className="mt-1"
                              type="checkbox"
                              checked={alertDraft.recipient_ids.includes(item.id ?? "")}
                              onChange={(e) => {
                                const id = item.id ?? "";
                                setAlertDraft({
                                  ...alertDraft,
                                  recipient_ids: e.target.checked ? [...alertDraft.recipient_ids, id] : alertDraft.recipient_ids.filter((value) => value !== id),
                                });
                              }}
                            />
                            <span>
                              {item.name}
                              {!item.active && <span className="ml-2 rounded bg-white/10 px-1.5 py-0.5 text-[0.65rem] uppercase text-amber-100">inativo</span>}
                              <span className="block text-xs text-emerald-50/58">{item.whatsapp}</span>
                            </span>
                          </label>
                        ))}
                      </div>
                    </Field>
                    <label className="flex items-start gap-2 rounded-md border border-white/10 p-2.5 text-sm">
                      <input className="mt-1" type="checkbox" checked={alertDraft.active} onChange={(e) => setAlertDraft({ ...alertDraft, active: e.target.checked })} />
                      <span>Ativar envio automatico<span className="block text-xs text-emerald-50/58">Deixe desmarcado para salvar sem disparar.</span></span>
                    </label>
                    {alertDraft.id && (
                      <button type="button" onClick={() => setAlertDraft(emptyAlert)} className="flex items-center justify-center gap-2 rounded-md border border-white/10 px-3 py-2 text-sm hover:bg-white/7">
                        <X size={16} /> Cancelar edicao
                      </button>
                    )}
                    <PrimaryButton><Plus size={16} /> {alertDraft.id ? "Salvar alerta" : "Criar alerta"}</PrimaryButton>
                  </form>
                </Panel>
                <Panel title="Alertas cadastrados">
                  <Rows items={alerts} getKey={(item) => item.id ?? item.name} render={(item) => (
                    <Row
                      title={item.name}
                      subtitle={`${item.deal_stage_name || item.deal_stage_id} - enviar apos ${item.time_threshold_hours}h - repetir ${item.repeat_interval_hours || 48}h - ${recipientSummary(item.recipient_ids ?? [])}`}
                      active={item.active}
                      onToggle={() => toggleAlert(item)}
                      onEdit={() => editAlert(item)}
                      onTest={() => runAlertDevOnly(item)}
                      testing={testingAlertID === item.id}
                    />
                  )} />
                </Panel>
              </div>
            </>
          )}

          {tab === "allowlist" && (
            <>
              <Header title="Allowlist do bot" subtitle="Numeros que podem conversar com o WhatsApp conectado ao bot." />
              <div className="grid gap-3 xl:grid-cols-[22rem_1fr]">
                <Panel title="Liberar numero">
                  <form onSubmit={submitAllowlist} className="grid gap-3">
                    <Field label="Numero autorizado" help="Quem nao estiver aqui sera ignorado pelo webhook.">
                      <input className="field" value={allowlistDraft.phone_number} onChange={(e) => setAllowlistDraft({ ...allowlistDraft, phone_number: e.target.value })} />
                    </Field>
                    <Field label="Rotulo">
                      <input className="field" value={allowlistDraft.label} onChange={(e) => setAllowlistDraft({ ...allowlistDraft, label: e.target.value })} />
                    </Field>
                    <Field label="Colaborador vinculado" help="Se informado, o perfil e o ID RD do colaborador serao usados no bot.">
                      <select className="field" value={allowlistDraft.collaborator_id} onChange={(e) => {
                        const c = collaborators.find((item) => item.id === e.target.value);
                        setAllowlistDraft({ ...allowlistDraft, collaborator_id: e.target.value, role: c?.role ?? allowlistDraft.role });
                      }}>
                        <option value="">Sem vinculo</option>
                        {collaborators.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
                      </select>
                    </Field>
                    <Field label="Perfil fallback">
                      <select className="field" value={allowlistDraft.role} onChange={(e) => setAllowlistDraft({ ...allowlistDraft, role: e.target.value as AllowlistEntry["role"] })}>
                        <option value="director">Diretoria</option>
                        <option value="supervisor">Supervisor</option>
                        <option value="seller">Vendedor</option>
                      </select>
                    </Field>
                    <PrimaryButton><Plus size={16} /> Salvar numero</PrimaryButton>
                  </form>
                </Panel>
                <Panel title="Numeros liberados">
                  <Rows items={allowlist} getKey={(item) => item.id ?? item.phone_number} render={(item) => (
                    <Row title={item.phone_number} subtitle={`${item.label || "Sem rotulo"} - ${roleLabel(item.role)}${item.sync_pending ? " - sync pendente" : ""}`} active={item.active} onToggle={() => toggleAllowlist(item)} />
                  )} />
                </Panel>
              </div>
            </>
          )}
        </section>
      </div>
    </main>
  );
}

function Header({ title, subtitle }: { title: string; subtitle: string }) {
  return (
    <header className="glass rounded-md px-4 py-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      <p className="mt-0.5 text-xs text-emerald-50/62">{subtitle}</p>
    </header>
  );
}

function NavButton({ active, icon, label, onClick }: { active: boolean; icon: React.ReactNode; label: string; onClick: () => void }) {
  return (
    <button onClick={onClick} className={`flex min-w-0 items-center gap-2 rounded-md px-2.5 py-2 text-left text-xs ${active ? "brand-active" : "text-emerald-50/75 hover:bg-white/7"}`}>
      {icon}
      <span className="truncate">{label}</span>
    </button>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="glass rounded-md p-3">
      <h3 className="mb-3 text-xs font-semibold uppercase text-emerald-50/68">{title}</h3>
      {children}
    </div>
  );
}

function Field({ label, help, children }: { label: string; help?: string; children: React.ReactNode }) {
  return (
    <label className="grid gap-1 text-sm">
      <span className="font-medium text-emerald-50">{label}</span>
      {children}
      {help && <span className="text-xs leading-relaxed text-emerald-50/58">{help}</span>}
    </label>
  );
}

function Metric({ title, value, icon }: { title: string; value: number; icon: React.ReactNode }) {
  return (
    <div className="glass flex items-center gap-3 rounded-md p-3">
      <div className="brand-icon-bg grid h-9 w-9 shrink-0 place-items-center rounded-md">{icon}</div>
      <div className="min-w-0">
        <p className="text-xl font-semibold leading-none">{value}</p>
        <p className="mt-1 truncate text-xs text-emerald-50/62">{title}</p>
      </div>
    </div>
  );
}

function PrimaryButton({ children, onClick, disabled }: { children: React.ReactNode; onClick?: () => void; disabled?: boolean }) {
  return <button onClick={onClick} disabled={disabled} className="brand-button flex min-h-9 items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-semibold disabled:opacity-60">{children}</button>;
}

function Rows<T>({ items, getKey, render }: { items: T[]; getKey: (item: T) => string; render: (item: T) => React.ReactNode }) {
  if (items.length === 0) {
    return <p className="rounded-md border border-white/10 px-3 py-5 text-center text-xs text-emerald-50/62">Nenhum registro ainda.</p>;
  }
  return <div className="grid gap-2">{items.map((item) => <div key={getKey(item)}>{render(item)}</div>)}</div>;
}

function isWhatsAppConnected(state?: string) {
  const normalized = (state || "").toLowerCase();
  return normalized === "open" || normalized === "connected";
}

function statusLabel(state?: string) {
  if (!state) return "Nao conectado";
  if (isWhatsAppConnected(state)) return "Conectado";
  if (state.toLowerCase() === "connecting") return "Conectando";
  if (state.toLowerCase() === "close") return "Desconectado";
  return state;
}

function normalizeQRImage(base64?: string) {
  if (!base64) return "";
  if (base64.startsWith("data:image")) return base64;
  return `data:image/png;base64,${base64}`;
}

function Row({
  title,
  subtitle,
  active,
  onToggle,
  onEdit,
  onTest,
  testing,
}: {
  title: string;
  subtitle: string;
  active: boolean;
  onToggle: () => void;
  onEdit?: () => void;
  onTest?: () => void;
  testing?: boolean;
}) {
  return (
    <div className="flex flex-col gap-2 rounded-md border border-white/10 bg-black/10 p-3 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0">
        <p className="truncate font-medium">{title}</p>
        <p className="break-words text-xs leading-relaxed text-emerald-50/58">{subtitle}</p>
      </div>
      <div className="flex shrink-0 flex-wrap gap-1.5">
        {onEdit && (
          <button onClick={onEdit} className="flex h-8 w-8 items-center justify-center rounded-md bg-white/10 text-emerald-50" title="Editar">
            <Edit3 size={15} />
          </button>
        )}
        {onTest && (
          <button onClick={onTest} disabled={testing} className="rounded-md border border-amber-200/35 bg-amber-300/12 px-2.5 py-1.5 text-xs font-semibold text-amber-100 disabled:opacity-50" title="Executar este alerta agora">
            {testing ? "Rodando" : "DevOnly"}
          </button>
        )}
        <button onClick={onToggle} className={`flex items-center justify-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs ${active ? "brand-active" : "bg-white/10 text-emerald-50"}`}>
          <Check size={15} />
          {active ? "Ativo" : "Inativo"}
        </button>
      </div>
    </div>
  );
}
