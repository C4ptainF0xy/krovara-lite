<script lang="ts">
  import { autoIdle, shareActivity, setAutoIdle, setShareActivity } from '$lib/stores/status';
  import { clearPresence } from '$lib/xmpp/presence';
  import { t as tr } from '$lib/i18n';

  type Toggle = {
    label: string;
    desc: string;
    value: boolean;
    onchange: (v: boolean) => void;
  };

  const toggles = $derived<Toggle[]>([
    {
      label: $tr('privacy.autoIdle'),
      desc: $tr('privacy.autoIdleDesc'),
      value: $autoIdle,
      onchange: setAutoIdle
    },
    {
      label: $tr('privacy.shareActivity'),
      desc: $tr('privacy.shareActivityDesc'),
      value: $shareActivity,
      onchange: (v: boolean) => {
        setShareActivity(v);
        if (!v) void clearPresence().catch(() => {});
      }
    }
  ]);
</script>

<div class="max-w-md space-y-6">
  <div>
    <h2 class="text-subtitle font-semibold text-content">{$tr('privacy.title')}</h2>
    <p class="mt-1 text-body text-muted">
      {$tr('privacy.hintBefore')}<a href="/app/settings/status" class="text-accent hover:underline">{$tr('privacy.invisible')}</a>.
    </p>
  </div>

  <div class="space-y-2">
    {#each toggles as t (t.label)}
      <label class="flex cursor-pointer items-center justify-between gap-4 rounded-lg border border-border p-3">
        <span class="min-w-0">
          <span class="block text-body text-content">{t.label}</span>
          <span class="block text-label text-muted">{t.desc}</span>
        </span>
        <input
          type="checkbox"
          checked={t.value}
          onchange={(e) => t.onchange(e.currentTarget.checked)}
          class="size-5 shrink-0 cursor-pointer accent-primary"
        />
      </label>
    {/each}
  </div>
</div>
