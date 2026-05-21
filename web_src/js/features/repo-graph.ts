import {toggleElemClass} from '../utils/dom.ts';
import {GET} from '../modules/fetch.ts';
import {fomanticQuery} from '../modules/fomantic/base.ts';
import {parseDom} from '../utils.ts';

const ATTR_GRAPH_HAS_NEXT = 'data-graph-has-next';
const ATTR_GRAPH_NEXT_PAGE = 'data-graph-next-page';

let graphInfiniteScrollObserver: IntersectionObserver | null = null;
let graphInfiniteScrollLoading = false;

function stripPageFromGraphURL() {
  const locUrl = new URL(window.location.href);
  if (!locUrl.searchParams.has('page')) return;
  locUrl.searchParams.delete('page');
  const qs = locUrl.searchParams.toString();
  window.history.replaceState(null, '', locUrl.pathname + (qs ? `?${qs}` : ''));
}

function syncGraphPaginationState(elGraphBody: HTMLElement) {
  const st = elGraphBody.querySelector<HTMLElement>('#git-graph-ajax-state');
  if (!st) return;
  elGraphBody.setAttribute(ATTR_GRAPH_HAS_NEXT, st.getAttribute(ATTR_GRAPH_HAS_NEXT) ?? '');
  elGraphBody.setAttribute(ATTR_GRAPH_NEXT_PAGE, st.getAttribute(ATTR_GRAPH_NEXT_PAGE) ?? '');
  st.remove();
}

function bindGraphInfiniteScroll(elGraphBody: HTMLElement) {
  graphInfiniteScrollObserver?.disconnect();
  graphInfiniteScrollObserver = null;

  const sentinel = elGraphBody.querySelector<HTMLElement>('#graph-scroll-sentinel');
  if (!sentinel) return;

  graphInfiniteScrollObserver = new IntersectionObserver(
    async (entries) => {
      if (!entries.some((e) => e.isIntersecting)) return;
      try {
        await loadNextGraphPage(elGraphBody);
      } catch (err) {
        console.error(err);
      }
    },
    {root: null, rootMargin: '120px', threshold: 0},
  );
  graphInfiniteScrollObserver.observe(sentinel);
}

async function loadNextGraphPage(elGraphBody: HTMLElement) {
  if (graphInfiniteScrollLoading) return;
  if ((elGraphBody.getAttribute(ATTR_GRAPH_HAS_NEXT) ?? '') !== 'true') return;
  const nextPage = elGraphBody.getAttribute(ATTR_GRAPH_NEXT_PAGE) ?? '';
  if (!nextPage) return;

  graphInfiniteScrollLoading = true;
  const indicator = elGraphBody.querySelector<HTMLElement>('#graph-loading-indicator');
  indicator?.classList.remove('tw-hidden');

  try {
    const fetchUrl = new URL(window.location.href);
    fetchUrl.searchParams.set('div-only', 'true');
    fetchUrl.searchParams.set('page', nextPage);

    const resp = await GET(fetchUrl.toString());
    const doc = parseDom(await resp.text(), 'text/html');
    const chunk = doc.querySelector<HTMLElement>('.graph-load-more-chunk');
    if (!chunk) {
      return;
    }

    const relChunk = chunk.querySelector<HTMLElement>('.rel-chunk');
    const left = elGraphBody.querySelector<HTMLElement>('#git-graph-left');
    const revList = elGraphBody.querySelector<HTMLElement>('#rev-list');
    if (relChunk && left) {
      left.append(relChunk);
    }

    const chunkList = chunk.querySelector<HTMLElement>('.graph-chunk-rev-list');
    if (chunkList && revList) {
      revList.append(...Array.from(chunkList.children));
    }

    elGraphBody.setAttribute(ATTR_GRAPH_HAS_NEXT, chunk.getAttribute(ATTR_GRAPH_HAS_NEXT) ?? '');
    elGraphBody.setAttribute(ATTR_GRAPH_NEXT_PAGE, chunk.getAttribute(ATTR_GRAPH_NEXT_PAGE) ?? '');
  } catch (err) {
    console.error('repo graph load more:', err);
  } finally {
    graphInfiniteScrollLoading = false;
    indicator?.classList.add('tw-hidden');
  }
}

export function initRepoGraphGit() {
  const graphContainer = document.querySelector<HTMLElement>('#git-graph-container');
  if (!graphContainer) return;

  stripPageFromGraphURL();

  const elGraphBody = document.querySelector<HTMLElement>('#git-graph-body');
  if (!elGraphBody) return;

  syncGraphPaginationState(elGraphBody);

  const url = new URL(window.location.href);
  const params = url.searchParams;

  const elColorMonochrome = document.querySelector<HTMLElement>('#flow-color-monochrome')!;
  const elColorColored = document.querySelector<HTMLElement>('#flow-color-colored')!;
  const toggleColorMode = (mode: 'monochrome' | 'colored') => {
    toggleElemClass(graphContainer, 'monochrome', mode === 'monochrome');
    toggleElemClass(graphContainer, 'colored', mode === 'colored');

    toggleElemClass(elColorMonochrome, 'active', mode === 'monochrome');
    toggleElemClass(elColorColored, 'active', mode === 'colored');

    const loc = new URL(window.location.href);
    loc.searchParams.set('mode', mode);
    window.history.replaceState(null, '', loc.pathname + (loc.search ? `${loc.search}` : ''));
    url.href = window.location.href;
  };
  elColorMonochrome.addEventListener('click', () => toggleColorMode('monochrome'));
  elColorColored.addEventListener('click', () => toggleColorMode('colored'));

  const loadGitGraph = async () => {
    params.delete('page');
    const queryString = params.toString();
    const ajaxUrl = new URL(url);
    ajaxUrl.searchParams.set('div-only', 'true');
    ajaxUrl.searchParams.delete('page');
    const loc = new URL(window.location.href);
    loc.search = queryString ? `?${queryString}` : '';
    window.history.replaceState(null, '', loc.pathname + loc.search);
    url.href = window.location.href;

    elGraphBody.classList.add('is-loading');
    try {
      const resp = await GET(ajaxUrl.toString());
      elGraphBody.innerHTML = await resp.text();
      syncGraphPaginationState(elGraphBody);
      bindGraphInfiniteScroll(elGraphBody);
    } finally {
      elGraphBody.classList.remove('is-loading');
    }
  };

  const dropdownSelected = params.getAll('branch');
  if (params.has('hide-pr-refs') && params.get('hide-pr-refs') === 'true') {
    dropdownSelected.splice(0, 0, '...flow-hide-pr-refs');
  }

  const $dropdown = fomanticQuery('#flow-select-refs-dropdown');
  $dropdown.dropdown({clearable: true});
  $dropdown.dropdown('set selected', dropdownSelected);
  // must add the callback after setting the selected items, otherwise each "selected" item will trigger the callback
  $dropdown.dropdown('setting', {
    onRemove(toRemove: string) {
      if (toRemove === '...flow-hide-pr-refs') {
        params.delete('hide-pr-refs');
      } else {
        const branches = params.getAll('branch');
        params.delete('branch');
        for (const branch of branches) {
          if (branch !== toRemove) {
            params.append('branch', branch);
          }
        }
      }
      (async () => {
        try {
          await loadGitGraph();
        } catch (err) {
          console.error(err);
        }
      })();
    },
    onAdd(toAdd: string) {
      if (toAdd === '...flow-hide-pr-refs') {
        params.set('hide-pr-refs', 'true');
      } else {
        params.append('branch', toAdd);
      }
      (async () => {
        try {
          await loadGitGraph();
        } catch (err) {
          console.error(err);
        }
      })();
    },
  });

  bindGraphInfiniteScroll(elGraphBody);
}
