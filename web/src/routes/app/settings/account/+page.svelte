<script lang="ts">
  import { goto } from '$app/navigation';
  import { Check, LogOut, AtSign, Mail } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { auth, clearSession } from '$lib/stores/auth';
  import { stop as stopXMPP } from '$lib/xmpp/client';
  import { t } from '$lib/i18n';
  import { Button, Input } from '$lib/ui';
  import QRCode from 'qrcode';

  const user = $derived($auth.user);

  let newEmail = $state('');
  let emailPassword = $state('');
  let emailBusy = $state(false);
  let emailErr = $state<string | null>(null);
  let emailSent = $state(false);

  async function changeEmail(e: Event) {
    e.preventDefault();
    emailErr = null;
    emailSent = false;
    emailBusy = true;
    try {
      await api('/api/me/email', {
        method: 'POST',
        body: { new_email: newEmail.trim(), current_password: emailPassword }
      });
      emailSent = true;
      newEmail = '';
      emailPassword = '';
    } catch (err) {
      emailErr = err instanceof Error ? err.message : $t('account.err.failed');
    } finally {
      emailBusy = false;
    }
  }

  let current = $state('');
  let next = $state('');
  let confirm = $state('');
  let busy = $state(false);
  let err = $state<string | null>(null);
  let done = $state(false);

  async function changePassword(e: Event) {
    e.preventDefault();
    err = null;
    done = false;
    if (next.length < 8) {
      err = $t('account.err.tooShort');
      return;
    }
    if (next !== confirm) {
      err = $t('account.err.mismatch');
      return;
    }
    busy = true;
    try {
      await api('/api/me/password', {
        method: 'PATCH',
        body: { current_password: current, new_password: next }
      });
      done = true;
      current = '';
      next = '';
      confirm = '';
      setTimeout(() => (done = false), 2500);
    } catch (e) {
      err = e instanceof Error ? e.message : $t('account.err.failed');
    } finally {
      busy = false;
    }
  }

  async function logout() {
    try {
      await api('/api/auth/logout', { method: 'POST' });
    } catch {}
    stopXMPP();
    clearSession();
    await goto('/login');
  }

  let setup2FASecret = $state<string | null>(null);
  let setup2FAQrData = $state<string | null>(null);
  let code2FA = $state('');
  let setup2FAErr = $state<string | null>(null);
  let backupCodes = $state<string[] | null>(null);

  async function begin2FASetup() {
    try {
      const res = await api<{ secret: string; url: string }>('/api/me/2fa/setup', { method: 'GET' });
      setup2FASecret = res.secret;
      setup2FAQrData = await QRCode.toDataURL(res.url, { margin: 0, width: 192 });
    } catch (e) {
      console.error(e);
    }
  }

  async function confirm2FASetup(e: Event) {
    e.preventDefault();
    setup2FAErr = null;
    try {
      const res = await api<{ backup_codes: string[] }>('/api/me/2fa/enable', {
        method: 'POST',
        body: { code: code2FA }
      });
      backupCodes = res.backup_codes;
      await api('/api/me', { method: 'GET' }).then(r => auth.update(s => ({ ...s, user: r as any })));
    } catch (e) {
      setup2FAErr = e instanceof Error ? e.message : 'Invalid code';
    }
  }

  async function disable2FA() {
    if (!window.confirm('Es-tu sûr de vouloir désactiver la 2FA ?')) return;
    try {
      await api('/api/me/2fa', { method: 'DELETE' });
      await api('/api/me', { method: 'GET' }).then(r => auth.update(s => ({ ...s, user: r as any })));
      setup2FASecret = null;
      backupCodes = null;
    } catch (e) {
      console.error(e);
    }
  }

  let deleteConfirmText = $state('');
  let deleteErr = $state<string | null>(null);
  let deleting = $state(false);
  const deleteConfirmed = $derived(
    !!user?.username && deleteConfirmText.trim().toLowerCase() === user.username.toLowerCase()
  );
  async function deleteAccount() {
    if (!deleteConfirmed || deleting) return;
    deleting = true;
    deleteErr = null;
    try {
      await api('/api/me', { method: 'DELETE' });
      await logout();
    } catch (e) {
      deleteErr = e instanceof Error ? e.message : 'Échec de la suppression du compte.';
    } finally {
      deleting = false;
    }
  }
</script>

<div class="space-y-10">
  <section>
    <h2 class="text-subtitle font-semibold text-content">{$t('account.title')}</h2>
    <dl class="mt-4 max-w-md divide-y divide-border rounded-lg border border-border">
      <div class="flex items-center justify-between gap-4 px-4 py-3">
        <dt class="flex items-center gap-1.5 text-label text-muted">
          <AtSign size={13} class="shrink-0" />
          <span>{$t('account.username')}</span>
        </dt>
        <dd class="truncate text-body text-content">@{user?.username}</dd>
      </div>
      <div class="flex items-center justify-between gap-4 px-4 py-3">
        <dt class="flex items-center gap-1.5 text-label text-muted">
          <Mail size={13} class="shrink-0" />
          <span>{$t('account.email')}</span>
        </dt>
        <dd class="truncate text-body text-content">{user?.email}</dd>
      </div>
      {#if user?.is_admin}
        <div class="flex items-center justify-between gap-4 px-4 py-3">
          <dt class="text-label text-muted">{$t('account.role')}</dt>
          <dd class="text-body text-accent">{$t('account.admin')}</dd>
        </div>
      {/if}
    </dl>
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">Changer d'adresse email</h2>
    <p class="mt-1 text-body text-muted">
      On envoie un lien de confirmation à la nouvelle adresse. Ton email ne change qu'après l'avoir confirmé.
    </p>
    {#if emailSent}
      <div class="mt-4 flex items-start gap-2 rounded-lg border border-success/40 bg-success/10 px-4 py-3 text-body text-success">
        <Check size={16} class="mt-0.5 shrink-0" />
        <span>Lien de confirmation envoyé. Vérifie ta nouvelle boîte mail (lien valable 2 heures).</span>
      </div>
    {:else}
      <form onsubmit={changeEmail} class="mt-4 space-y-4">
        <Input
          label="Nouvelle adresse email"
          type="email"
          autocomplete="email"
          placeholder="nouvelle@adresse.com"
          bind:value={newEmail}
        />
        <Input
          label={$t('account.pw.current')}
          type="password"
          autocomplete="current-password"
          bind:value={emailPassword}
        />
        {#if emailErr}<p class="text-label text-danger">{emailErr}</p>{/if}
        <Button type="submit" loading={emailBusy} disabled={!newEmail.trim() || !emailPassword}>
          <Mail size={16} /> Envoyer le lien de confirmation
        </Button>
      </form>
    {/if}
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">{$t('account.pw.title')}</h2>
    <p class="mt-1 text-body text-muted">
      {$t('account.pw.hint')}
    </p>
    <form onsubmit={changePassword} class="mt-4 space-y-4">
      <Input
        label={$t('account.pw.current')}
        type="password"
        autocomplete="current-password"
        bind:value={current}
      />
      <Input
        label={$t('account.pw.new')}
        type="password"
        autocomplete="new-password"
        minlength={8}
        bind:value={next}
      />
      <Input
        label={$t('account.pw.confirm')}
        type="password"
        autocomplete="new-password"
        bind:value={confirm}
      />
      {#if err}<p class="text-label text-danger">{err}</p>{/if}
      <div class="flex items-center gap-3">
        <Button type="submit" loading={busy}>{$t('account.pw.submit')}</Button>
        {#if done}
          <span class="flex items-center gap-1.5 text-label text-success">
            <Check size={16} /> {$t('account.pw.done')}
          </span>
        {/if}
      </div>
    </form>
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">Authentification à deux facteurs (2FA)</h2>
    <p class="mt-1 text-body text-muted">Protège ton compte avec une couche de sécurité supplémentaire.</p>

    <div class="mt-4">
      {#if user?.totp_enabled}
        <div class="rounded-lg border border-success/40 bg-success/10 px-4 py-3">
          <p class="text-body text-success font-medium flex items-center gap-2">
            <Check size={16} /> 2FA activée
          </p>
          <Button type="button" variant="danger" class="mt-3" onclick={disable2FA}>
            Désactiver la 2FA
          </Button>
        </div>
      {:else}
        {#if !setup2FASecret}
          <Button type="button" onclick={begin2FASetup}>
            Activer la 2FA
          </Button>
        {:else if !backupCodes}
          <div class="space-y-4 rounded-lg border border-border bg-overlay p-4">
            <p class="text-body text-content">1. Scanne ce QR code avec ton application (ex: Google Authenticator, Authy).</p>
            <div class="bg-white p-2 w-fit rounded-md">
              {#if setup2FAQrData}
                <img src={setup2FAQrData} alt="QR Code" class="size-48" />
              {:else}
                <div class="size-48 bg-muted animate-pulse rounded-md"></div>
              {/if}
            </div>
            <p class="text-label text-muted">Clé secrète : <span class="font-mono bg-elevated px-1 rounded">{setup2FASecret}</span></p>

            <p class="text-body text-content mt-4">2. Saisis le code généré par ton application.</p>
            <form onsubmit={confirm2FASetup} class="flex gap-2">
              <Input
                label="Code à 6 chiffres"
                type="text"
                placeholder="123456"
                bind:value={code2FA}
                class="w-full tracking-widest font-mono"
              />
              <div class="mt-6">
                <Button type="submit">Activer</Button>
              </div>
            </form>
            {#if setup2FAErr}<p class="text-label text-danger">{setup2FAErr}</p>{/if}
          </div>
        {:else}
          <div class="space-y-4 rounded-lg border border-success/40 bg-success/10 p-4">
            <p class="text-body text-success font-medium flex items-center gap-2">
              <Check size={16} /> 2FA activée avec succès !
            </p>
            <p class="text-body text-content mt-4">Conserve précieusement ces codes de secours. Ils te permettront d'accéder à ton compte si tu perds ton téléphone.</p>
            <div class="grid grid-cols-2 gap-2 bg-elevated p-3 rounded-md font-mono text-sm text-content">
              {#each backupCodes as code}
                <div>{code}</div>
              {/each}
            </div>
            <Button type="button" onclick={() => { setup2FASecret = null; backupCodes = null; }}>
              J'ai sauvegardé ces codes
            </Button>
          </div>
        {/if}
      {/if}
    </div>
  </section>

  <section class="max-w-md">
    <h2 class="text-subtitle font-semibold text-content">{$t('account.session.title')}</h2>
    <p class="mt-1 text-body text-muted">{$t('account.session.hint')}</p>
    <div class="mt-4">
      <Button type="button" variant="danger" onclick={logout}>
        <LogOut size={16} /> {$t('account.session.logout')}
      </Button>
    </div>
  </section>

  <section class="max-w-md border-t border-danger/20 pt-6">
    <h2 class="text-subtitle font-semibold text-danger">Supprimer le compte</h2>
    <p class="mt-1 text-body text-muted">
      La suppression de ton compte est irréversible. Tes messages seront conservés mais anonymisés sous le nom "Deleted User" (conformité RGPD).
    </p>
    <div class="mt-4 space-y-4 rounded-lg border border-danger/30 bg-danger/5 p-4">
      <p class="text-sm text-content">Pour confirmer, saisis ton nom d'utilisateur <strong>{user?.username}</strong> :</p>
      <Input
        label=""
        type="text"
        placeholder={user?.username}
        bind:value={deleteConfirmText}
      />
      {#if deleteErr}<p class="text-label text-danger">{deleteErr}</p>{/if}
      <Button type="button" variant="danger" onclick={deleteAccount} loading={deleting} disabled={!deleteConfirmed}>
        Supprimer définitivement mon compte
      </Button>
    </div>
  </section>
</div>
