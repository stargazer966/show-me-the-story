<script>
  import { currentPage } from './lib/router.js';
  import { progress, taskRunning, contextPage, toastStore, currentProject, projectLanguage, config, settings, chatSessions, currentChatSession } from './lib/stores.js';
  import { connectSSE } from './lib/sse.js';
  import { api } from './lib/api.js';
  import { onMount, onDestroy } from 'svelte';
  import { t, uiLocale, setLocale } from './lib/i18n/index.js';
  import TaskTokenBadge from './components/TaskTokenBadge.svelte';
  import Projects from './pages/Projects.svelte';
  import History from './pages/History.svelte';
  import Config from './pages/Config.svelte';
  import Outline from './pages/StoryDesk.svelte';
  import Writing from './pages/Writing.svelte';
  import Proofread from './pages/Proofread.svelte';
  import Relations from './pages/Relations.svelte';
  import Skills from './pages/Skills.svelte';
  import Foreshadows from './pages/Foreshadows.svelte';
  import Memory from './pages/Memory.svelte';
  import ChatPanel from './components/ChatPanel.svelte';
  import ConfirmModal from './components/ConfirmModal.svelte';
  import StorageErrorModal from './components/StorageErrorModal.svelte';

  let chatPanel;
  let initializing = true;
  let restoring = false;
  let restoreTimer;
  let destroyed = false;
  let navigationOpen = false;
  let navigationDesktop = globalThis.localStorage?.getItem('navigation-desktop-visible') !== 'false';
  let assistantOpen = false;
  let assistantDesktop = globalThis.localStorage?.getItem("assistant-desktop-visible") !== "false";
  let wideWorkspace = false;
  $: assistantVisible = wideWorkspace ? assistantDesktop : assistantOpen;
  let navigationButton;
  let assistantButton;

  $: if ($taskRunning && !$currentProject) restoreCurrentProject();

  onDestroy(() => { destroyed = true; clearTimeout(restoreTimer); });

  async function restoreCurrentProject() {
    if (restoring || destroyed) return;
    restoring = true;
    try {
      const cur = await api('GET', '/api/projects/current');
      if (destroyed) return;
      if (cur.name && cur.name !== $currentProject) {
        config.set(null);
        progress.set(null);
        settings.set(null);
        chatSessions.set([]);
        currentChatSession.set(null);
        currentProject.set(cur.name);
        initializing = false;
        if (cur.language) {
          projectLanguage.set(cur.language);
          setLocale(cur.language);
        }
        if ($taskRunning) currentPage.set('writing');
        await Promise.allSettled([
          api('GET', '/api/config').then(config.set),
          api('GET', '/api/progress').then(progress.set),
          api('GET', '/api/settings').then(settings.set),
          api('GET', '/api/chat/sessions').then(chatSessions.set),
        ]);
      }
      initializing = false;
    } catch (_) {
      clearTimeout(restoreTimer);
      if (!destroyed) restoreTimer = setTimeout(restoreCurrentProject, 2000);
    } finally {
      restoring = false;
    }
  }

  let appVersion = '';
  let latestVersion = '';
  let hasUpdate = false;
  const latestReleaseURL = 'https://github.com/Nigh/show-me-the-story/releases/latest';

  $: $contextPage = $currentPage;

  onMount(async () => {
    restoreCurrentProject();
    connectSSE();
    // Fetch app version
    try {
      const ver = await api('GET', '/api/version');
      appVersion = ver.version || 'dev';
    } catch (e) {}
    // Check for updates (skip for dev builds)
    if (appVersion && appVersion !== 'dev') {
      try {
        const resp = await fetch('https://api.github.com/repos/Nigh/show-me-the-story/releases/latest');
        if (resp.ok) {
          const data = await resp.json();
          latestVersion = data.tag_name || '';
          if (latestVersion && latestVersion !== appVersion) {
            hasUpdate = true;
          }
        }
      } catch (e) {}
    }
  });

  $: chapterStats = (() => {
    const chs = ($progress?.chapters || []).filter(c => !c.inherited && c.outline?.trim());
    if (chs.length === 0) return '';
    const accepted = chs.filter(c => c.status === 'accepted').length;
    return $t('app.chapters.written', { accepted, total: chs.length });
  })();

  async function sendToChat(text) {
    if (chatPanel) {
      openAssistant();
      await chatPanel.sendMessageToChat(text);
    }
  }

  async function backToProjects() {
    if ($taskRunning || restoring) return;
    try {
      const status = await api('GET', '/api/status');
      if (status.is_task_running || $taskRunning) {
        taskRunning.set(true);
        return;
      }
    } catch (_) { return; }
    currentProject.set(null);
    config.set(null);
  }

  function toggleLocale() {
    setLocale($uiLocale === 'en' ? 'zh' : 'en');
  }

  function toggleNavigation() {
    if (desktopNavigation) {
      navigationDesktop = !navigationDesktop;
      try { localStorage.setItem('navigation-desktop-visible', String(navigationDesktop)); } catch {}
      return;
    }
    if (navigationOpen) { closeNavigation(); return; }
    navigationOpen = true;
    requestAnimationFrame(() => document.querySelector('#workspace-navigation button')?.focus());
  }
  function openAssistant() {
    if (wideWorkspace) {
      assistantDesktop = true;
      localStorage.setItem('assistant-desktop-visible','true');
    } else assistantOpen = true;
    requestAnimationFrame(() => document.querySelector('#writing-assistant button')?.focus());
  }
  function closeNavigation() {
    if (!navigationOpen) return;
    navigationOpen = false;
    requestAnimationFrame(() => navigationButton?.focus());
  }
  function closeAssistant() {
    if (wideWorkspace) {
      assistantDesktop = false;
      localStorage.setItem('assistant-desktop-visible','false');
    }
    assistantOpen = false;
    requestAnimationFrame(() => assistantButton?.focus());
  }
  function goTo(page) {
    window.location.hash = '#' + page;
    closeNavigation();
  }
  function handleShellKeydown(event) {
    if (event.key === 'Escape') { closeNavigation(); if(assistantOpen && !wideWorkspace)closeAssistant(); return; }
    if (event.key !== 'Tab') return;
    const panel = document.getElementById(navigationOpen ? 'workspace-navigation' : assistantOpen && !wideWorkspace ? 'writing-assistant' : '');
    if (!panel) return;
    const controls = [...panel.querySelectorAll('button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])')];
    if (!controls.length) return;
    const first = controls[0], last = controls[controls.length - 1];
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  }
  let workspaceWidth;
  $: desktopNavigation = workspaceWidth >= 1024;
  $: navigationVisible = desktopNavigation ? navigationDesktop : navigationOpen;
  $: if (desktopNavigation) navigationOpen = false;
  $: wideWorkspace=workspaceWidth>=1280;
</script>

<svelte:window bind:innerWidth={workspaceWidth} on:keydown={handleShellKeydown} />

<div class="app-shell flex flex-col bg-base-300 text-base-content overflow-hidden">
  <!-- Header -->
  <header class="app-header navbar bg-base-200 border-b border-base-content/10 px-3 min-h-14 shrink-0 gap-2">
    {#if $currentProject}
      <button bind:this={navigationButton} class="btn btn-ghost btn-sm" on:click={toggleNavigation} aria-expanded={navigationVisible} aria-controls="workspace-navigation" aria-label={$t(navigationVisible?'app.navigation.hide':'app.navigation.open')} title={$t(navigationVisible?'app.navigation.hide':'app.navigation.open')}>
        <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="4" width="18" height="16" rx="2"/>
          <path d="M9 4v16"/>
          {#if navigationVisible}<path d="m15 9-3 3 3 3"/>{:else}<path d="m12 9 3 3-3 3"/>{/if}
        </svg>
      </button>
    {/if}
    <span class="app-brand text-base font-semibold whitespace-nowrap">{$t('app.title')}</span>
    {#if appVersion}
      <span class="badge badge-xs badge-ghost font-mono">{appVersion}</span>
    {/if}
    {#if hasUpdate}
      <a href={latestReleaseURL} target="_blank" rel="noopener" class="badge badge-xs badge-warning gap-0.5 no-underline">
        {$t('app.newVersion')}
      </a>
    {/if}
    {#if $currentProject}
      <span class="app-project-title min-w-0 text-sm font-medium leading-snug">{$config?.story?.title?.trim() || $t('app.untitled')}</span>
      <span class="badge badge-sm badge-outline uppercase max-sm:hidden" title={$t($projectLanguage === 'en' ? 'common.lang.en' : 'common.lang.zh')}>
        {$projectLanguage === 'en' ? 'EN' : 'ZH'}
      </span>
      <button
        class="btn btn-ghost btn-xs gap-1"
        on:click={backToProjects}
        disabled={$taskRunning || restoring}
        title={$taskRunning ? $t('app.switchProject.disabled') : $t('app.switchProject.tooltip')}
      >
        {$t('app.switchProject')}
      </button>
      {#if chapterStats}
        <span class="badge badge-sm badge-ghost max-md:hidden">{chapterStats}</span>
      {/if}
      {#if $taskRunning}
        <span class="badge badge-sm badge-warning gap-1">
          <span class="loading loading-spinner loading-xs"></span>
          {$t('app.aiThinking')}
          <TaskTokenBadge className="badge badge-xs badge-warning font-mono border-0" />
        </span>
      {/if}
    {/if}
    <span class="flex-1"></span>
    <button
      class="btn btn-ghost btn-xs gap-1"
      on:click={toggleLocale}
      title={$t('app.uiLang.label')}
    >
      {$uiLocale === 'en' ? $t('app.uiLang.en') : $t('app.uiLang.zh')}
    </button>
    {#if $currentProject}
      <button bind:this={assistantButton} class="btn btn-outline btn-sm" on:click={()=>assistantVisible?closeAssistant():openAssistant()} aria-expanded={assistantVisible} aria-controls="writing-assistant">{$t(assistantVisible?'app.hideAssistant':'app.assistant')}</button>
    {/if}
  </header>

  {#if initializing || ($taskRunning && !$currentProject)}
    <main class="flex-1 flex items-center justify-center" aria-busy="true">
      <span class="loading loading-spinner loading-lg"></span>
    </main>
  {:else if !$currentProject}
    <!-- Project selection -->
    <main class="project-picker flex-1 overflow-y-auto p-4 sm:p-8">
      <Projects />
    </main>
  {:else}
    <div class="workspace flex flex-1 overflow-hidden">
      {#if navigationOpen || (assistantOpen && !wideWorkspace)}
        <button class="workspace-scrim" aria-label={$t('common.close')} on:click={() => { if (navigationOpen) closeNavigation(); if (assistantOpen) closeAssistant(); }}></button>
      {/if}
      <!-- Left: vertical nav -->
      <nav id="workspace-navigation" class:drawer-open={navigationOpen} class:desktop-hidden={!navigationDesktop} class="app-navigation flex flex-col w-48 shrink-0 bg-base-200 border-r border-base-content/10 py-4 px-3 gap-1" aria-label={$t('app.navigation')}>
        <button class="btn btn-ghost btn-sm mb-2 lg:hidden" on:click={closeNavigation}>{$t('common.close')}</button>
        {#each [
          ['config', 'nav.config'],
          ['outline', 'nav.outline'],
          ['writing', 'nav.writing'],
          ['proofread', 'nav.proofread']
        ] as [page, labelKey]}
          <button class="nav-item btn btn-sm justify-start w-full px-3 text-sm {$currentPage === page ? 'btn-primary font-medium' : 'btn-ghost'}"
            aria-current={$currentPage === page ? 'page' : undefined} on:click={() => goTo(page)}>{$t(labelKey)}</button>
        {/each}
        <details class="mt-3" open={['history','foreshadows','memory','relations','skills'].includes($currentPage)}>
          <summary class="btn btn-sm btn-ghost w-full justify-start">{$t('app.tools')} <span aria-hidden="true">▾</span></summary>
          {#each [
          ['history','flow.history'],
          ['foreshadows', 'nav.foreshadows'],
          ['memory', 'nav.memory'],
          ['relations', 'nav.relations'],
          ['skills', 'nav.skills']
        ] as [page, labelKey]}
          <button
            class="nav-item btn btn-sm justify-start w-full px-3 text-sm {$currentPage === page ? 'btn-primary font-medium' : 'btn-ghost'}"
            aria-current={$currentPage === page ? 'page' : undefined}
            on:click={() => goTo(page)}
          >
            {$t(labelKey)}
          </button>
          {/each}
        </details>
      </nav>

      <!-- Center: page content -->
      <main class="workspace-main @container flex-[2] min-w-0 overflow-y-auto p-3 sm:p-5 xl:border-r xl:border-base-content/10">
        {#if $currentPage === 'config'}
          <Config {sendToChat} />
        {:else if $currentPage === 'outline'}
          {#key $currentProject}<Outline />{/key}
        {:else if $currentPage === 'writing'}
          <Writing />
        {:else if $currentPage === 'history'}
          <History />
        {:else if $currentPage === 'proofread'}
          <Proofread />
        {:else if $currentPage === 'foreshadows'}
          <Foreshadows />
        {:else if $currentPage === 'memory'}
          <Memory />
        {:else if $currentPage === 'relations'}
          <Relations />
        {:else if $currentPage === 'skills'}
          <Skills />
        {/if}
      </main>

      <!-- Right: Chat Panel -->
      <aside id="writing-assistant" class:drawer-open={assistantOpen} class:desktop-hidden={!assistantDesktop} class="assistant-panel flex-1 min-w-72 max-w-md bg-base-200 overflow-hidden" aria-label={$t('app.assistant')}>
        <button class="btn btn-ghost btn-sm m-2 mb-0" on:click={closeAssistant}>{$t('app.hideAssistant')}</button>
        <ChatPanel bind:this={chatPanel} contextPage={$currentPage} />
      </aside>
    </div>
  {/if}

  <!-- Toasts -->
  <div class="fixed top-5 right-5 z-50 flex flex-col gap-2">
    {#each $toastStore as t (t.id)}
      <div class="alert alert-sm {t.type === 'success' ? 'alert-success' : t.type === 'error' ? 'alert-error' : 'alert-info'} toast-enter  max-w-sm">
        <span>{t.msg}</span>
      </div>
    {/each}
  </div>

  <ConfirmModal />
  <StorageErrorModal />
</div>
