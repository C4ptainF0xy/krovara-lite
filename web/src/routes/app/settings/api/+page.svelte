<script lang="ts">
  import { onMount } from 'svelte';
  import { KeyRound, Plus, Trash2, Copy, Check, Webhook } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { t as tr } from '$lib/i18n';
  import { Button, Input } from '$lib/ui';

  type Lang = 'node' | 'python' | 'go';
  let hookLang = $state<Lang>('node');
  const HOOK_SNIPPETS: Record<Lang, string> = {
    node: `import crypto from "node:crypto";

// rawBody = corps brut de la requête (Buffer/string), AVANT parsing JSON.
function verify(rawBody, header, secret) {
  const expected = "sha256=" +
    crypto.createHmac("sha256", secret).update(rawBody).digest("hex");
  const a = Buffer.from(header);
  const b = Buffer.from(expected);
  return a.length === b.length && crypto.timingSafeEqual(a, b);
}

// header = req.headers["x-krovara-signature"]`,
    python: `import hmac, hashlib

def verify(raw_body: bytes, header: str, secret: str) -> bool:
    expected = "sha256=" + hmac.new(
        secret.encode(), raw_body, hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, header)

# header = request.headers["X-Krovara-Signature"]`,
    go: `import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

func verify(rawBody []byte, header, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(rawBody)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(header))
}`
  };
  let hookCopied = $state(false);
  async function copyHook() {
    try {
      await navigator.clipboard.writeText(HOOK_SNIPPETS[hookLang]);
      hookCopied = true;
      setTimeout(() => (hookCopied = false), 1200);
    } catch {
    }
  }

  type Token = {
    id: string;
    name: string;
    prefix: string;
    scopes: string[];
    last_used_at?: string;
    token?: string;
  };

  let tokens = $state<Token[]>([]);
  let loading = $state(true);

  let newName = $state('');
  let scopeRead = $state(true);
  let scopeWrite = $state(false);
  let busy = $state(false);
  let err = $state<string | null>(null);
  let justCreated = $state<string | null>(null);
  let copied = $state(false);

  onMount(load);
  async function load() {
    loading = true;
    try {
      tokens = await api<Token[]>('/api/me/api-tokens');
    } finally {
      loading = false;
    }
  }

  async function create(e: Event) {
    e.preventDefault();
    const name = newName.trim();
    const scopes = [scopeRead && 'read', scopeWrite && 'write'].filter(Boolean) as string[];
    if (!name || scopes.length === 0) {
      err = 'Nom et au moins un scope requis.';
      return;
    }
    busy = true;
    err = null;
    try {
      const t = await api<Token>('/api/me/api-tokens', { method: 'POST', body: { name, scopes } });
      justCreated = t.token ?? null;
      newName = '';
      await load();
    } catch (e2) {
      err = e2 instanceof Error ? e2.message : 'Échec';
    } finally {
      busy = false;
    }
  }

  async function copy() {
    if (!justCreated) return;
    try {
      await navigator.clipboard.writeText(justCreated);
      copied = true;
      setTimeout(() => (copied = false), 1500);
    } catch {
    }
  }

  async function revoke(t: Token) {
    if (!confirm(`Révoquer le jeton « ${t.name} » ? Les intégrations qui l'utilisent cesseront de fonctionner.`)) return;
    await api(`/api/me/api-tokens/${t.id}`, { method: 'DELETE' });
    tokens = tokens.filter((x) => x.id !== t.id);
  }
</script>

<div class="space-y-8">
  <section>
    <h2 class="flex items-center gap-2 text-subtitle font-semibold text-content">
      <KeyRound size={20} /> {$tr('api.tokens.title')}
    </h2>
    <p class="mt-1 text-body text-muted">
      {$tr('api.tokens.hint')}
    </p>
  </section>

  {#if justCreated}
    <div class="rounded-lg border border-success/40 bg-success/5 p-4">
      <p class="text-label font-medium text-success">{$tr('api.tokens.created')}</p>
      <div class="mt-2 flex gap-2">
        <input
          readonly
          value={justCreated}
          class="h-10 flex-1 rounded border border-border bg-base/50 px-3 font-mono text-label text-content outline-none"
        />
        <Button type="button" onclick={copy}>
          {#if copied}<Check size={16} /> {$tr('common.copied')}{:else}<Copy size={16} /> {$tr('common.copy')}{/if}
        </Button>
      </div>
      <button type="button" onclick={() => (justCreated = null)} class="mt-2 text-label text-muted hover:text-content">
        {$tr('api.tokens.hideCopied')}
      </button>
    </div>
  {/if}

  <form onsubmit={create} class="max-w-md space-y-3 rounded-lg border border-border p-4">
    <Input label={$tr('api.tokens.name')} bind:value={newName} placeholder={$tr('api.tokens.namePlaceholder')} maxlength={64} />
    <div class="space-y-1.5">
      <span class="block text-label font-medium text-muted">{$tr('api.tokens.permissions')}</span>
      <label class="flex cursor-pointer items-center gap-2 text-body text-content">
        <input type="checkbox" bind:checked={scopeRead} class="accent-primary" /> {$tr('api.tokens.read')}
      </label>
      <label class="flex cursor-pointer items-center gap-2 text-body text-content">
        <input type="checkbox" bind:checked={scopeWrite} class="accent-primary" /> {$tr('api.tokens.write')}
      </label>
    </div>
    {#if err}<p class="text-label text-danger">{err}</p>{/if}
    <Button type="submit" loading={busy}><Plus size={16} /> {$tr('api.tokens.generate')}</Button>
  </form>

  <section>
    <h3 class="mb-2 text-label font-semibold uppercase tracking-wide text-muted">{$tr('api.tokens.active')}</h3>
    {#if loading}
      <div class="space-y-2">{#each [0, 1] as i (i)}<div class="h-14 animate-pulse rounded-lg bg-elevated/50"></div>{/each}</div>
    {:else if tokens.length === 0}
      <p class="text-body text-muted">{$tr('api.tokens.empty')}</p>
    {:else}
      <div class="space-y-1">
        {#each tokens as t (t.id)}
          <div class="group flex items-center gap-3 rounded-lg border border-border p-3">
            <span class="min-w-0 flex-1">
              <span class="block truncate text-body text-content">{t.name}</span>
              <span class="block font-mono text-label text-muted">{t.prefix}…· {t.scopes.join(', ')}</span>
            </span>
            <button
              type="button"
              title={$tr('api.tokens.revoke')}
              onclick={() => revoke(t)}
              class="grid size-8 shrink-0 place-items-center rounded text-muted opacity-0 transition group-hover:opacity-100 hover:text-danger"
            >
              <Trash2 size={15} />
            </button>
          </div>
        {/each}
      </div>
    {/if}
  </section>

  <section class="border-t border-border pt-8">
    <h2 class="flex items-center gap-2 text-subtitle font-semibold text-content">
      <Webhook size={20} /> Vérifier les webhooks
    </h2>
    <p class="mt-1 text-body text-muted">
      Chaque livraison de webhook est signée. Vérifie l'en-tête
      <code class="rounded bg-base px-1.5 py-0.5 font-mono text-label text-accent">X-Krovara-Signature</code>
      avant de traiter la requête.
    </p>

    <ul class="mt-3 space-y-1 text-label text-muted">
      <li>• <code class="font-mono text-content">X-Krovara-Signature: sha256=&lt;hex&gt;</code> : HMAC-SHA256 du corps brut, clé = le secret du webhook.</li>
      <li>• <code class="font-mono text-content">X-Krovara-Event</code> : nom de l'événement.</li>
      <li>• <code class="font-mono text-content">X-Krovara-Webhook-Id</code> : identifiant du webhook.</li>
    </ul>
    <p class="mt-2 text-label text-warning">
      Calcule la signature sur le <strong>corps brut</strong> (avant tout parsing JSON) et compare en temps constant.
    </p>

    <div class="mt-4 overflow-hidden rounded-lg border border-border">
      <div class="flex items-center justify-between border-b border-border bg-elevated/40 px-2 py-1.5">
        <div class="flex gap-1">
          {#each [{ k: 'node', l: 'Node.js' }, { k: 'python', l: 'Python' }, { k: 'go', l: 'Go' }] as t (t.k)}
            <button
              type="button"
              onclick={() => (hookLang = t.k as Lang)}
              class="rounded px-2.5 py-1 text-label font-medium transition-colors duration-150
                     {hookLang === t.k ? 'bg-primary text-white' : 'text-muted hover:text-content'}"
            >
              {t.l}
            </button>
          {/each}
        </div>
        <button
          type="button"
          onclick={copyHook}
          class="flex items-center gap-1.5 rounded px-2 py-1 text-label text-muted transition-colors hover:text-content"
        >
          {#if hookCopied}<Check size={14} class="text-success" /> {$tr('common.copied')}{:else}<Copy size={14} /> {$tr('common.copy')}{/if}
        </button>
      </div>
      <pre class="overflow-x-auto bg-base p-4 font-mono text-label leading-relaxed text-content"><code>{HOOK_SNIPPETS[hookLang]}</code></pre>
    </div>
  </section>
</div>
