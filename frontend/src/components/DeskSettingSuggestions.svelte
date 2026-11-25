<script>
  import { t } from '../lib/i18n/index.js';
  import { settings } from '../lib/stores.js';

  export let suggestions = [];
  export let decisions = {};
  export let part;
  export let version;
  export let adopted;
  export let busy;
  export let decide;

  let editing = null;
  let value = '';
  $: pending = suggestions.filter((_, i) => !decisions[decisionKey(part, version, i)]).length;
  const decisionKey = (p, v, i) => [p, v, i + 1].join(':');

  function characterName(id, entries) {
    return entries?.find(character => character.id === id)?.name || id;
  }

  async function apply(i, nextValue) {
    if (await decide('apply_setting', i + 1, nextValue)) editing = null;
  }
</script>

{#if suggestions.length}
  <details open class="rounded-box border border-base-content/15 bg-base-100">
    <summary class="cursor-pointer px-4 py-3 font-semibold">{$t('desk.settingSuggestions', {count: pending})}</summary>
    <div class="px-4 pb-4 space-y-3">
      {#if !adopted}<p class="text-sm">{$t('desk.settingUseVersion')}</p>{/if}
      {#each suggestions as suggestion, i}
        {@const status = decisions[decisionKey(part, version, i)]}
        {@const field = suggestion.field || (suggestion.kind === 'characters' ? 'background' : suggestion.kind === 'worldview' ? 'description' : 'label')}
        <article class="rounded-box border border-base-content/15 p-3 space-y-2">
          <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h3 class="font-semibold break-words">
              {#if suggestion.kind === 'relations'}
                {characterName(suggestion.source_id, $settings?.characters)} → {characterName(suggestion.target_id, $settings?.characters)}
              {:else}
                {suggestion.name}
              {/if}
            </h3>
            <span class="text-sm text-base-content/75">{$t('desk.setting.' + suggestion.kind)} · {$t(suggestion.entity_id ? 'desk.settingUpdate' : 'desk.settingNew')}</span>
            {#if suggestion.category}<span class="text-sm">· {$t('config.wv.cat.' + suggestion.category)}</span>{/if}
          </div>
          <div class="text-sm">
            <span class="font-medium">{$t(suggestion.origin === 'draft' ? 'desk.settingDraft' : 'desk.settingAI', {beat: suggestion.beat})}</span>
            <blockquote class="border-s-2 border-base-content/20 ps-2 whitespace-pre-wrap break-words">{suggestion.quote}</blockquote>
          </div>
          <dl class="grid gap-2 sm:grid-cols-2">
            {#if suggestion.entity_id}
              <div><dt class="text-sm font-medium">{$t('facts.before')} · {$t('facts.field.' + field)}</dt><dd class="whitespace-pre-wrap break-words">{suggestion.before || '—'}</dd></div>
            {/if}
            <div><dt class="text-sm font-medium">{$t('desk.settingProposed')} · {$t('facts.field.' + field)}</dt><dd class="whitespace-pre-wrap break-words">{suggestion.after}</dd></div>
          </dl>
          {#if status}
            <div class="flex flex-wrap items-center gap-2">
              <span class="badge badge-outline">{$t('desk.settingStatus.' + status)}</span>
              {#if status === 'applied'}<button class="btn btn-outline btn-sm" disabled={busy} on:click={() => decide('undo_setting', i + 1)}>{$t('desk.settingUndo')}</button>{/if}
            </div>
          {:else if adopted}
            {#if editing === i}
              <form class="space-y-2" on:submit|preventDefault={() => apply(i, value)}>
                <label class="block">{$t('desk.settingProposed')}<textarea class="textarea w-full mt-1" rows="3" bind:value disabled={busy}></textarea></label>
                <div class="flex gap-2">
                  <button class="btn btn-primary btn-sm" type="submit" disabled={busy || !value.trim()}>{$t('desk.settingApply')}</button>
                  <button class="btn btn-ghost btn-sm" type="button" disabled={busy} on:click={() => editing = null}>{$t('common.cancel')}</button>
                </div>
              </form>
            {:else}
              <div class="flex flex-wrap gap-2">
                <button class="btn btn-primary btn-sm" disabled={busy} on:click={() => apply(i, undefined)}>{$t('desk.settingApply')}</button>
                <button class="btn btn-outline btn-sm" disabled={busy} on:click={() => { editing = i; value = suggestion.after; }}>{$t('desk.settingEdit')}</button>
                <button class="btn btn-ghost btn-sm" disabled={busy} on:click={() => decide('dismiss_setting', i + 1)}>{$t('desk.settingDismiss')}</button>
              </div>
            {/if}
          {/if}
        </article>
      {/each}
    </div>
  </details>
{/if}
