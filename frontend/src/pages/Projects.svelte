<script>
  import { onMount } from 'svelte';
  import { api, apiFetch } from '../lib/api.js';
  import { currentProject, projects, addToast, showConfirm, taskRunning, progress, config, settings, chatSessions, currentChatSession, projectLanguage, selectedChapter } from '../lib/stores.js';
  import { t, setLocale } from '../lib/i18n/index.js';

  let newProjectName = '';
  let newProjectLang = 'en';
  let creating = false;
  let backupBusy = false;
  let restoreName = '';
  let restoreFiles;
  let restoreInput;
  let createDialog, restoreDialog, templateDialog;
  let createMode = 'blank';
  let selecting = '';
  let createError = '', restoreError = '', templateError = '';
  let savingTemplate = false;
  let loading = true;
  let loadError = '';
  let recents = [];

  $: orderedProjects = [...$projects].sort((a, b) => recentRank(a.name, recents) - recentRank(b.name, recents));

  function recentRank(name, names) {
    const index = names.indexOf(name);
    return index === -1 ? names.length : index;
  }

  function rememberProject(name) {
    recents = [name, ...recents.filter(item => item !== name)].slice(0, 50);
    try { localStorage.setItem('recent-story-projects', JSON.stringify(recents)); } catch {}
  }

  function showCreate(mode = 'blank') {
    createMode = mode;
    createError = '';
    createDialog.showModal();
  }

  function showTemplate(name) {
    source = name;
    templateError = '';
    templateName = name;
    templateDialog.showModal();
  }

  let templates=[], template='', source='', templateName='', premise='', characters=true, worldview=true;
  $: chosen=templates.find(t=>t.name===template);
  async function loadTemplates(){templates=await api('GET','/api/templates')}
  onMount(async () => {
    try {
      const names = JSON.parse(localStorage.getItem('recent-story-projects'));
      if (Array.isArray(names)) recents = names;
    } catch {}
    await Promise.all([loadProjects(), loadTemplates().catch(e => addToast(e.message, 'error'))]);
  });
  async function saveTemplate() {
    if (savingTemplate || !templateName.trim()) return;
    savingTemplate = true;
    templateError = '';
    try {
      const name = templateName.trim();
      await api('POST', '/api/templates', { source, name, characters, worldview });
      templateDialog.close();
      await loadTemplates().catch(e => addToast(e.message, 'error'));
      template = name;
    } catch (e) { templateError = e.message; }
    finally { savingTemplate = false; }
  }

  async function loadProjects() {
    loadError = '';
    try {
      const list = await api('GET', '/api/projects');
      projects.set(Array.isArray(list) ? list : []);
    } catch (e) {
      loadError = e.message;
    } finally { loading = false; }
  }

  async function selectProject(name) {
    if (selecting || $taskRunning) return;
    const project = $projects.find((item) => item.name === name);
    if (project && project.compatibility !== 'supported') {
      const message = project.compatible_app_line
        ? $t('projects.incompatible.message', { format: project.project_format, line: project.compatible_app_line, version: project.recommended_app_version })
        : $t('projects.incompatible.unknown');
      showConfirm(message, () => {});
      return;
    }
    selecting = name;
    try {
      await api('POST', '/api/projects/select', { name });
      rememberProject(name);
      config.set(null);
      progress.set(null);
      selectedChapter.set(-1);
      currentProject.set(name);
      // Reload all project data
      try { progress.set(await api('GET', '/api/progress')); } catch (e) {}
      try {
        const cfg = await api('GET', '/api/config');
        config.set(cfg);
        if (cfg && cfg.language) {
          projectLanguage.set(cfg.language);
          setLocale(cfg.language);
        }
      } catch (e) {}
      try { settings.set(await api('GET', '/api/settings')); } catch (e) {}
      try { chatSessions.set(await api('GET', '/api/chat/sessions')); } catch (e) {}
      currentChatSession.set(null);
      addToast($t('projects.toast.switched', { name }), 'success');
    } catch (e) {
      addToast(e.message, 'error');
    } finally { selecting = ''; }
  }

  async function createProject() {
    if (creating || $taskRunning || (createMode === 'template' && !chosen)) return;
    creating = true;
    createError = '';
    try {
      const {name} = await api('POST', '/api/projects', { name: newProjectName.trim(), language: createMode === 'template' ? chosen.config.language : newProjectLang, template: createMode === 'template' ? template : '', premise });
      newProjectName = ''; premise='';
      createDialog.close();
      await loadProjects();
      await selectProject(name);
    } catch (e) {
      createError = e.message;
    } finally {
      creating = false;
    }
  }

  async function deleteProject(name) {
    showConfirm($t('projects.confirm.delete', { name }), async () => {
      try {
        await api('DELETE', '/api/projects/' + encodeURIComponent(name));
        await loadProjects();
        addToast($t('projects.toast.deleted'), 'success');
      } catch (e) {
        addToast(e.message, 'error');
      }
    });
  }

  async function backupProject(name) {
    backupBusy = true;
    try {
      const response = await apiFetch('/api/projects/' + encodeURIComponent(name) + '/backup');
      const url = URL.createObjectURL(await response.blob());
      const link = document.createElement('a');
      link.href = url;
      link.download = `${name}-${new Date().toISOString().slice(0, 10)}.zip`;
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) { addToast(e.message, 'error'); }
    finally { backupBusy = false; }
  }

  async function restoreProject() {
    if (!restoreName.trim() || !restoreFiles?.length) return;
    backupBusy = true;
    restoreError = '';
    try {
      await apiFetch('/api/projects/restore?name=' + encodeURIComponent(restoreName.trim()), {
        method: 'POST', headers: { 'Content-Type': 'application/zip' }, body: restoreFiles[0],
      });
      restoreName = '';
      restoreInput.value = '';
      restoreFiles = null;
      restoreDialog?.close();
      await loadProjects();
      addToast($t('projects.restore.done'), 'success');
    } catch (e) { restoreError = e.message; }
    finally { backupBusy = false; }
  }

</script>

<section class="project-home mx-auto max-w-4xl space-y-5" aria-labelledby="projects-heading">
  <header class="flex flex-wrap items-center justify-between gap-3">
    <h1 id="projects-heading" class="text-2xl font-semibold">{$t('projects.title')}</h1>
    <div class="flex flex-wrap gap-2">
      <button class="btn btn-outline" disabled={backupBusy || $taskRunning} on:click={() => { restoreError = ''; restoreDialog.showModal(); }}>{$t('projects.restore.short')}</button>
      <button class="btn btn-primary" disabled={$taskRunning} on:click={() => showCreate()}>＋ {$t('projects.create')}</button>
    </div>
  </header>

  {#if loading}
    <p role="status" class="p-4">{$t('projects.loading')}</p>
  {:else if loadError}
    <p role="alert" class="alert alert-error">{loadError}</p>
    <button class="btn btn-outline" on:click={loadProjects}>{$t('projects.retry')}</button>
  {:else if !orderedProjects.length}
    <div class="card bg-base-100 border border-base-content/10">
      <div class="card-body items-start">
        <p>{$t('projects.empty')}</p>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-primary" on:click={() => showCreate()}>{$t('wf.createBlank')}</button>
          {#if templates.length}<button class="btn btn-outline" on:click={() => showCreate('template')}>{$t('wf.createTemplate')}</button>{/if}
        </div>
      </div>
    </div>
  {:else}
    <ul class="project-list rounded-box bg-base-100 border border-base-content/10 divide-y divide-base-content/10">
      {#each orderedProjects as p (p.name)}
        <li class="project-row">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
              <h2 class="font-semibold break-words min-w-0">{p.name}</h2>
              <span class="badge badge-outline badge-sm">{p.language === 'en' ? 'EN' : 'ZH'}</span>
              {#if p.book_status === 'completed' && p.compatibility === 'supported'}
                <span class="badge badge-success badge-sm">{$t('projects.phase.completed')}</span>
              {/if}
            </div>
            {#if p.title && p.title !== p.name}<p class="text-sm break-words">{p.title}</p>{/if}
            <p class="text-sm text-base-content/70">
              {#if p.compatibility !== 'supported'}
                {$t('projects.incompatible.badge')} · {$t('projects.incompatible.hint')}
              {:else if Number(p.outlined_chapters) || Number(p.written_chapters)}
                {$t('projects.progress', { outlined: p.outlined_chapters, written: p.written_chapters })}
              {:else if Number(p.draft_chapters)}
                {$t('projects.drafts', { count: p.draft_chapters })}
              {:else}
                {$t('projects.emptyProject')}
              {/if}
            </p>
          </div>
          <div class="flex items-center gap-2">
            <button class="btn btn-sm btn-outline" disabled={!!selecting || $taskRunning}
              aria-label={$t(p.compatibility === 'supported' ? 'projects.openNamed' : 'projects.detailsNamed', { name: p.name })}
              on:click={() => selectProject(p.name)}>
              {$t(p.compatibility === 'supported' ? 'projects.open' : 'projects.details')}
            </button>
            <details class="project-actions" name="project-actions">
              <summary class="btn btn-ghost btn-sm" aria-label={$t('projects.actions', { name: p.name })}>⋯</summary>
              <div class="project-actions-panel rounded-box border border-base-content/15 bg-base-100 p-1 shadow-md">
                {#if p.compatibility === 'supported'}
                  <button class="btn btn-ghost btn-sm justify-start" disabled={backupBusy || $taskRunning} on:click={event => { event.currentTarget.closest('details').open = false; backupProject(p.name); }}>{$t('projects.backup')}</button>
                  <button class="btn btn-ghost btn-sm justify-start" on:click={event => { event.currentTarget.closest('details').open = false; showTemplate(p.name); }}>{$t('wf.saveTemplate')}</button>
                {/if}
                <button class="btn btn-ghost btn-sm justify-start text-error" disabled={$taskRunning || backupBusy} on:click={event => { event.currentTarget.closest('details').open = false; deleteProject(p.name); }}>{$t('projects.delete')}</button>
              </div>
            </details>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<dialog bind:this={createDialog} class="modal" aria-labelledby="project-create-heading" on:cancel={event => { if (creating) event.preventDefault(); }}>
  <form class="modal-box space-y-4" on:submit|preventDefault={createProject}>
    <h2 id="project-create-heading" class="text-lg font-semibold">{$t('projects.create')}</h2>
    {#if createError}<p class="alert alert-error" role="alert">{createError}</p>{/if}
    <fieldset disabled={creating} class="space-y-4">
      <div class="grid grid-cols-2 gap-2">
        <label class="btn {createMode === 'blank' ? 'btn-outline border-primary' : 'btn-ghost'}">
          <input class="radio radio-sm" type="radio" name="project-mode" value="blank" bind:group={createMode} />
          {$t('wf.blank')}
        </label>
        <label class="btn {createMode === 'template' ? 'btn-outline border-primary' : 'btn-ghost'}">
          <input class="radio radio-sm" type="radio" name="project-mode" value="template" bind:group={createMode} />
          {$t('projects.fromTemplate')}
        </label>
      </div>
      {#if createMode === 'template'}
        {#if templates.length}
          <label class="block">{$t('wf.template')}
            <select class="select w-full mt-1" bind:value={template}>
              <option value="" disabled>{$t('projects.chooseTemplate')}</option>
              {#each templates as item}<option value={item.name}>{item.name}</option>{/each}
            </select>
          </label>
          {#if chosen}<p class="text-sm">{$t('projects.create.lang')} · {chosen.config.language.toUpperCase()} · {chosen.config.story.target_words_per_chapter} {$t('wf.words')}</p>{/if}
        {:else}
          <p class="text-sm">{$t('projects.noTemplates')}</p>
        {/if}
      {:else}
        <label class="block">{$t('projects.create.lang')}
          <select class="select w-full mt-1" bind:value={newProjectLang}>
            <option value="en">{$t('common.lang.en')}</option>
            <option value="zh">{$t('common.lang.zh')}</option>
          </select>
        </label>
        <p class="text-sm text-base-content/70">{$t('projects.create.langHint')}</p>
      {/if}
      <details class="space-y-2">
        <summary class="cursor-pointer">{$t('wf.projectDetails')}</summary>
        <label class="block">{$t('projects.name')}<input class="input w-full mt-1" bind:value={newProjectName} /></label>
        <label class="block">{$t('wf.premise')}<textarea class="textarea w-full mt-1" bind:value={premise}></textarea></label>
      </details>
    </fieldset>
    <div class="modal-action flex-wrap">
      <button type="button" class="btn btn-ghost" disabled={creating} on:click={() => createDialog.close()}>{$t('common.cancel')}</button>
      <button type="submit" class="btn btn-primary" disabled={creating || $taskRunning || (createMode === 'template' && !chosen)}>
        {#if creating}<span class="loading loading-spinner loading-xs" aria-hidden="true"></span>{/if}
        {$t(createMode === 'template' ? 'wf.createTemplate' : 'wf.createBlank')}
      </button>
    </div>
  </form>
</dialog>

<dialog bind:this={restoreDialog} class="modal" aria-labelledby="project-restore-heading" on:cancel={event => { if (backupBusy) event.preventDefault(); }}>
  <form class="modal-box space-y-4" on:submit|preventDefault={restoreProject}>
    <h2 id="project-restore-heading" class="text-lg font-semibold">{$t('projects.restore.title')}</h2>
    {#if restoreError}<p class="alert alert-error" role="alert">{restoreError}</p>{/if}
    <fieldset disabled={backupBusy || $taskRunning} class="space-y-3">
      <label class="block">{$t('projects.restore.file')}<input type="file" accept=".zip,application/zip" class="file-input w-full mt-1" bind:this={restoreInput} bind:files={restoreFiles} /></label>
      <label class="block">{$t('projects.restore.name')}<input class="input w-full mt-1" bind:value={restoreName} /></label>
      <details class="text-sm text-base-content/75">
        <summary class="cursor-pointer">{$t('projects.restore.about')}</summary>
        <p class="mt-2">{$t('projects.restore.hint')}</p>
      </details>
    </fieldset>
    <div class="modal-action flex-wrap">
      <button type="button" class="btn btn-ghost" disabled={backupBusy} on:click={() => restoreDialog.close()}>{$t('common.cancel')}</button>
      <button class="btn btn-primary" type="submit" disabled={backupBusy || $taskRunning || !restoreName.trim() || !restoreFiles?.length}>
        {#if backupBusy}<span class="loading loading-spinner loading-xs" aria-hidden="true"></span>{/if}
        {$t('projects.restore.button')}
      </button>
    </div>
  </form>
</dialog>

<dialog bind:this={templateDialog} class="modal" aria-labelledby="project-template-heading" on:cancel={event => { if (savingTemplate) event.preventDefault(); }}>
  <form class="modal-box space-y-3" on:submit|preventDefault={saveTemplate}>
    <h2 id="project-template-heading" class="text-lg font-semibold break-words">{$t('wf.saveTemplate')} · {source}</h2>
    {#if templateError}<p class="alert alert-error" role="alert">{templateError}</p>{/if}
    <label class="block">{$t('wf.template')}<input class="input w-full mt-1" bind:value={templateName}/></label>
    <label class="flex items-center gap-2"><input class="checkbox checkbox-sm" type="checkbox" bind:checked={characters}/> {$t('config.char.title')}</label>
    <label class="flex items-center gap-2"><input class="checkbox checkbox-sm" type="checkbox" bind:checked={worldview}/> {$t('wf.scenes')}</label>
    <div class="modal-action">
      <button type="button" class="btn btn-ghost" disabled={savingTemplate} on:click={() => templateDialog.close()}>{$t('common.cancel')}</button>
      <button type="submit" class="btn btn-primary" disabled={savingTemplate || !templateName.trim()}>{$t('common.save')}</button>
    </div>
  </form>
</dialog>

<style>
  .project-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: 1rem;
    padding: 1rem;
  }
  .project-actions { position: relative; }
  .project-actions summary { list-style: none; }
  .project-actions summary::-webkit-details-marker { display: none; }
  .project-actions-panel {
    position: absolute;
    inset-inline-end: 0;
    z-index: 2;
    min-width: max-content;
    display: grid;
  }
  .modal-box { overscroll-behavior: contain; }
  @media (max-width: 400px) {
    .project-row { grid-template-columns: minmax(0, 1fr); gap: .5rem; }
    .project-actions-panel { inset-inline: 0 auto; }
  }
</style>
