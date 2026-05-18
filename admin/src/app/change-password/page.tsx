"use client";

import { useState } from "react";
import { changePassword } from "@/lib/api";

export default function ChangePasswordPage() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (next !== confirm) {
      setError("A confirmação não bate com a nova senha.");
      return;
    }
    try {
      await changePassword(current, next);
      window.location.href = "/";
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha ao trocar a senha");
    }
  }

  return (
    <main className="grid min-h-screen place-items-center px-4">
      <form onSubmit={submit} className="glass w-full max-w-md rounded-lg p-6">
        <div className="mb-6 flex items-center gap-3">
          <div className="grid h-10 w-10 place-items-center overflow-hidden rounded-lg bg-white/92 p-1.5">
            <img src="/synova-logo.png" alt="Synova" className="max-h-full max-w-full object-contain" />
          </div>
          <div>
            <h1 className="text-lg font-semibold">Trocar senha</h1>
            <p className="text-sm text-emerald-50/62">Obrigatório no primeiro acesso</p>
          </div>
        </div>
        <label className="mb-3 block text-sm">Senha atual<input className="field mt-1" type="password" value={current} onChange={(e) => setCurrent(e.target.value)} /></label>
        <label className="mb-3 block text-sm">Nova senha<input className="field mt-1" type="password" value={next} onChange={(e) => setNext(e.target.value)} /></label>
        <label className="mb-4 block text-sm">Confirmar nova senha<input className="field mt-1" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} /></label>
        {error && <p className="mb-3 rounded-md border border-red-300/30 bg-red-500/12 px-3 py-2 text-sm text-red-100">{error}</p>}
        <button className="brand-button w-full rounded-lg px-4 py-2.5 font-semibold">Salvar senha</button>
      </form>
    </main>
  );
}
