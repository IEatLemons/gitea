<script lang="ts">
import {defineComponent, nextTick} from 'vue';
import {SvgIcon} from '../svg.ts';
import {GET} from '../modules/fetch.ts';
import {buildNavbarOrganizationsUrl, buildNavbarRepoSearchUrl} from '../features/navbar-quick-access-utils.ts';

const {appSubUrl} = window.config;
const quickAccessLimit = 8;

type NavbarQuickAccessTab = 'repos' | 'organizations';

type NavbarQuickAccessRepo = {
  id: number,
  link: string,
  full_name: string,
  fork: boolean,
  mirror: boolean,
  template: boolean,
  private: boolean,
  internal: boolean,
};

type NavbarQuickAccessOrg = {
  name: string,
  full_name: string,
  num_repos: number,
  visibility: string,
  link: string,
};

type NavbarQuickAccessRepoSearchResult = {
  repository: NavbarQuickAccessRepo,
};

type NavbarQuickAccessRepoSearchResponse = {
  data: NavbarQuickAccessRepoSearchResult[],
};

type NavbarQuickAccessOrgSearchResponse = {
  data: NavbarQuickAccessOrg[],
};

function getRoot() {
  return document.querySelector<HTMLElement>('#navbar-quick-access')!;
}

export default defineComponent({
  components: {SvgIcon},

  data() {
    const root = getRoot();
    return {
      uid: Number(root.dataset.uid!),
      canCreateOrganization: root.dataset.canCreateOrganization === 'true',
      textQuickAccess: root.dataset.textQuickAccess!,
      textRepository: root.dataset.textRepository!,
      textOrganization: root.dataset.textOrganization!,
      textSearchRepos: root.dataset.textSearchRepos!,
      textSearchOrgs: root.dataset.textSearchOrgs!,
      textNoRepo: root.dataset.textNoRepo!,
      textNoOrg: root.dataset.textNoOrg!,
      textNewRepo: root.dataset.textNewRepo!,
      textNewOrg: root.dataset.textNewOrg!,
      textLoading: root.dataset.textLoading!,
      textOrgVisibilityLimited: root.dataset.textOrgVisibilityLimited!,
      textOrgVisibilityPrivate: root.dataset.textOrgVisibilityPrivate!,
      subUrl: appSubUrl,

      isOpen: false,
      tab: 'repos' as NavbarQuickAccessTab,
      searchQuery: '',
      activeIndex: -1,

      repos: [] as NavbarQuickAccessRepo[],
      reposTotalCount: null as number | null,
      reposLoaded: false,
      reposLoading: false,
      reposLoadedQuery: null as string | null,
      reposLoadingQuery: null as string | null,
      reposRequestID: 0,

      organizations: [] as NavbarQuickAccessOrg[],
      organizationsTotalCount: null as number | null,
      organizationsLoaded: false,
      organizationsLoading: false,
      organizationsLoadedQuery: null as string | null,
      organizationsLoadingQuery: null as string | null,
      organizationsRequestID: 0,
    };
  },

  computed: {
    searchPlaceholder() {
      return this.tab === 'repos' ? this.textSearchRepos : this.textSearchOrgs;
    },

    isLoading() {
      return this.tab === 'repos' ? this.reposLoading : this.organizationsLoading;
    },

    isCurrentTabEmpty() {
      if (this.isLoading) return false;
      return this.tab === 'repos' ? this.repos.length === 0 : this.organizations.length === 0;
    },

    currentTotalCount() {
      return this.tab === 'repos' ? this.reposTotalCount : this.organizationsTotalCount;
    },
  },

  mounted() {
    document.addEventListener('click', this.onDocumentClick);
    document.addEventListener('keydown', this.onDocumentKeydown);
  },

  beforeUnmount() {
    document.removeEventListener('click', this.onDocumentClick);
    document.removeEventListener('keydown', this.onDocumentKeydown);
  },

  methods: {
    toggleOpen() {
      this.isOpen = !this.isOpen;
      if (this.isOpen) {
        this.ensureCurrentTabLoaded();
        this.focusSearchInput();
      }
    },

    close() {
      this.isOpen = false;
      this.activeIndex = -1;
    },

    changeTab(tab: NavbarQuickAccessTab) {
      this.tab = tab;
      this.activeIndex = -1;
      this.ensureCurrentTabLoaded();
      this.focusSearchInput();
    },

    focusSearchInput() {
      nextTick(() => {
        const input = getRoot().querySelector<HTMLInputElement>('.navbar-quick-access-search-input');
        if (input) input.focus({preventScroll: true});
      });
    },

    ensureCurrentTabLoaded() {
      if (this.tab === 'repos') {
        if (
          (!this.reposLoaded || this.reposLoadedQuery !== this.searchQuery) &&
          this.reposLoadingQuery !== this.searchQuery
        ) {
          this.searchRepos();
        }
      } else if (
        (!this.organizationsLoaded || this.organizationsLoadedQuery !== this.searchQuery) &&
        this.organizationsLoadingQuery !== this.searchQuery
      ) {
        this.searchOrganizations();
      }
    },

    onSearchInput() {
      this.activeIndex = -1;
      if (this.tab === 'repos') {
        this.searchRepos();
      } else {
        this.searchOrganizations();
      }
    },

    async searchRepos() {
      const requestID = ++this.reposRequestID;
      const searchQuery = this.searchQuery;
      this.reposLoading = true;
      this.reposLoadingQuery = searchQuery;
      let response: Response;
      let json: NavbarQuickAccessRepoSearchResponse;
      try {
        response = await GET(buildNavbarRepoSearchUrl(appSubUrl, this.uid, searchQuery, quickAccessLimit));
        if (!response.ok) {
          if (requestID === this.reposRequestID) {
            this.reposLoading = false;
            this.reposLoadingQuery = null;
          }
          return;
        }
        json = await response.json() as NavbarQuickAccessRepoSearchResponse;
      } catch {
        if (requestID === this.reposRequestID) {
          this.reposLoading = false;
          this.reposLoadingQuery = null;
        }
        return;
      }
      if (requestID !== this.reposRequestID) return;
      const totalCount = response.headers.get('X-Total-Count');
      this.repos = json.data.map((repoSearchResult) => repoSearchResult.repository);
      this.reposTotalCount = totalCount ? Number(totalCount) : this.repos.length;
      this.reposLoaded = true;
      this.reposLoadedQuery = searchQuery;
      this.reposLoading = false;
      this.reposLoadingQuery = null;
    },

    async searchOrganizations() {
      const requestID = ++this.organizationsRequestID;
      const searchQuery = this.searchQuery;
      this.organizationsLoading = true;
      this.organizationsLoadingQuery = searchQuery;
      let response: Response;
      let json: NavbarQuickAccessOrgSearchResponse;
      try {
        response = await GET(buildNavbarOrganizationsUrl(appSubUrl, searchQuery, quickAccessLimit));
        if (!response.ok) {
          if (requestID === this.organizationsRequestID) {
            this.organizationsLoading = false;
            this.organizationsLoadingQuery = null;
          }
          return;
        }
        json = await response.json() as NavbarQuickAccessOrgSearchResponse;
      } catch {
        if (requestID === this.organizationsRequestID) {
          this.organizationsLoading = false;
          this.organizationsLoadingQuery = null;
        }
        return;
      }
      if (requestID !== this.organizationsRequestID) return;
      const totalCount = response.headers.get('X-Total-Count');
      this.organizations = json.data;
      this.organizationsTotalCount = totalCount ? Number(totalCount) : this.organizations.length;
      this.organizationsLoaded = true;
      this.organizationsLoadedQuery = searchQuery;
      this.organizationsLoading = false;
      this.organizationsLoadingQuery = null;
    },

    repoIcon(repo: NavbarQuickAccessRepo) {
      if (repo.fork) return 'octicon-repo-forked';
      if (repo.mirror) return 'octicon-mirror';
      if (repo.template) return 'octicon-repo-template';
      if (repo.private) return 'octicon-lock';
      if (repo.internal) return 'octicon-repo';
      return 'octicon-repo';
    },

    orgDisplayName(org: NavbarQuickAccessOrg) {
      return org.full_name ? `${org.full_name} (${org.name})` : org.name;
    },

    orgVisibilityText(org: NavbarQuickAccessOrg) {
      if (org.visibility === 'limited') return this.textOrgVisibilityLimited;
      if (org.visibility === 'private') return this.textOrgVisibilityPrivate;
      return '';
    },

    onInputKeydown(e: KeyboardEvent) {
      if (e.isComposing) return;
      switch (e.key) {
        case 'ArrowDown':
          e.preventDefault();
          this.moveActive(1);
          break;
        case 'ArrowUp':
          e.preventDefault();
          this.moveActive(-1);
          break;
        case 'Enter':
          this.openActive();
          break;
        case 'Escape':
          this.close();
          break;
      }
    },

    moveActive(delta: number) {
      const itemCount = this.tab === 'repos' ? this.repos.length : this.organizations.length;
      if (!itemCount) return;
      if (this.activeIndex === -1) {
        this.activeIndex = delta > 0 ? 0 : itemCount - 1;
        return;
      }
      this.activeIndex = (this.activeIndex + delta + itemCount) % itemCount;
    },

    openActive() {
      if (this.activeIndex === -1) this.activeIndex = 0;
      let link = '';
      if (this.tab === 'repos' && this.repos[this.activeIndex]) {
        link = this.repos[this.activeIndex].link;
      } else if (this.tab === 'organizations' && this.organizations[this.activeIndex]) {
        link = this.organizations[this.activeIndex].link;
      }
      if (link) window.location.assign(link);
    },

    onDocumentClick(e: Event) {
      if (!this.isOpen) return;
      const target = e.target as Node;
      if (!getRoot().contains(target)) this.close();
    },

    onDocumentKeydown(e: KeyboardEvent) {
      if (this.isOpen && e.key === 'Escape') this.close();
    },
  },
});
</script>

<template>
  <button
    type="button"
    class="navbar-quick-access-toggle"
    :aria-expanded="isOpen"
    aria-controls="navbar-quick-access-menu"
    @click="toggleOpen"
  >
    <svg-icon name="octicon-repo" :size="16"/>
    <span>{{ textQuickAccess }}</span>
    <svg-icon name="octicon-triangle-down" :size="14" class="navbar-quick-access-caret"/>
  </button>
  <div v-if="isOpen" id="navbar-quick-access-menu" class="navbar-quick-access-menu">
    <div class="navbar-quick-access-tabs" role="tablist">
      <button
        type="button"
        class="navbar-quick-access-tab"
        :class="{active: tab === 'repos'}"
        role="tab"
        :aria-selected="tab === 'repos'"
        @click="changeTab('repos')"
      >
        <svg-icon name="octicon-repo" :size="16"/>
        <span>{{ textRepository }}</span>
        <span v-if="reposTotalCount !== null" class="navbar-quick-access-count">{{ reposTotalCount }}</span>
      </button>
      <button
        type="button"
        class="navbar-quick-access-tab"
        :class="{active: tab === 'organizations'}"
        role="tab"
        :aria-selected="tab === 'organizations'"
        @click="changeTab('organizations')"
      >
        <svg-icon name="octicon-organization" :size="16"/>
        <span>{{ textOrganization }}</span>
        <span v-if="organizationsTotalCount !== null" class="navbar-quick-access-count">{{ organizationsTotalCount }}</span>
      </button>
    </div>
    <div class="navbar-quick-access-search">
      <div class="ui small fluid left icon input">
        <input
          ref="searchInput"
          v-model="searchQuery"
          class="navbar-quick-access-search-input"
          type="search"
          spellcheck="false"
          maxlength="255"
          :placeholder="searchPlaceholder"
          @input="onSearchInput"
          @keydown="onInputKeydown"
        >
        <i class="icon loading-icon-3px" :class="{'is-loading': isLoading}">
          <svg-icon name="octicon-search" :size="16"/>
        </i>
      </div>
    </div>
    <div class="navbar-quick-access-list">
      <div v-if="isLoading && currentTotalCount === null" class="navbar-quick-access-empty">{{ textLoading }}</div>
      <ul v-else-if="tab === 'repos' && repos.length" class="navbar-quick-access-items">
        <li
          v-for="(repo, index) in repos"
          :key="repo.id"
          class="navbar-quick-access-item"
          :class="{active: index === activeIndex}"
          @mouseenter="activeIndex = index"
        >
          <a :href="repo.link" class="navbar-quick-access-link">
            <svg-icon :name="repoIcon(repo)" :size="16" class="navbar-quick-access-item-icon"/>
            <span class="navbar-quick-access-item-name">{{ repo.full_name }}</span>
          </a>
        </li>
      </ul>
      <ul v-else-if="tab === 'organizations' && organizations.length" class="navbar-quick-access-items">
        <li
          v-for="(org, index) in organizations"
          :key="org.name"
          class="navbar-quick-access-item"
          :class="{active: index === activeIndex}"
          @mouseenter="activeIndex = index"
        >
          <a :href="org.link" class="navbar-quick-access-link">
            <svg-icon name="octicon-organization" :size="16" class="navbar-quick-access-item-icon"/>
            <span class="navbar-quick-access-item-name">{{ orgDisplayName(org) }}</span>
            <span v-if="org.visibility !== 'public'" class="ui tiny basic label navbar-quick-access-visibility">
              {{ orgVisibilityText(org) }}
            </span>
            <span class="navbar-quick-access-meta">
              {{ org.num_repos }}
              <svg-icon name="octicon-repo" :size="14"/>
            </span>
          </a>
        </li>
      </ul>
      <div v-else-if="isCurrentTabEmpty" class="navbar-quick-access-empty">
        <svg-icon :name="tab === 'repos' ? 'octicon-git-branch' : 'octicon-organization'" :size="24"/>
        <span>{{ tab === 'repos' ? textNoRepo : textNoOrg }}</span>
      </div>
    </div>
    <div class="navbar-quick-access-footer">
      <a v-if="tab === 'repos'" :href="`${subUrl}/repo/create`" class="navbar-quick-access-create-link">
        <svg-icon name="octicon-plus" :size="16"/>
        <span>{{ textNewRepo }}</span>
      </a>
      <a v-else-if="canCreateOrganization" :href="`${subUrl}/org/create`" class="navbar-quick-access-create-link">
        <svg-icon name="octicon-plus" :size="16"/>
        <span>{{ textNewOrg }}</span>
      </a>
    </div>
  </div>
</template>
