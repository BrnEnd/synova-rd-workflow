"use client";

import { useState } from "react";
import { LogIn } from "lucide-react";
import { login } from "@/lib/api";

export default function LoginPage() {
  const [email, setEmail] = useState("admin@synova.local");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      const result = await login(email, password);
      window.location.href = result.must_change_password ? "/change-password" : "/";
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha no login");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="grid min-h-screen place-items-center px-4">
      <form onSubmit={submit} className="glass w-full max-w-sm rounded-lg p-6">
        <div className="mb-6 flex items-center gap-3">
          <div className="grid h-10 w-10 place-items-center overflow-hidden rounded-lg bg-white/92 p-1.5">
            <img src="/synova-logo.png" alt="Synova" className="max-h-full max-w-full object-contain" />
          </div>
          <div>
            <h1 className="text-lg font-semibold">Synova Admin</h1>
            <p className="text-sm text-emerald-50/62">Acesso operacional</p>
          </div>
        </div>
        <label className="mb-3 block text-sm">
          E-mail
          <input className="field mt-1" value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        <label className="mb-4 block text-sm">
          Senha
          <input className="field mt-1" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        {error && <p className="mb-3 rounded-md border border-red-300/30 bg-red-500/12 px-3 py-2 text-sm text-red-100">{error}</p>}
        <button disabled={loading} className="brand-button flex w-full items-center justify-center gap-2 rounded-lg px-4 py-2.5 font-semibold disabled:opacity-60">
          <LogIn size={17} />
          {loading ? "Entrando..." : "Entrar"}
        </button>
      </form>
    </main>
  );
}
