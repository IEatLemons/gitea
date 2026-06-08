import {createCodeEditor} from '../modules/codeeditor/main.ts';
import {onInputDebounce, queryElems, toggleElem} from '../utils/dom.ts';
import {POST} from '../modules/fetch.ts';
import {initRepoSettingsBranchesDrag} from './repo-settings-branches.ts';
import {fomanticQuery} from '../modules/fomantic/base.ts';
import {globMatch} from '../utils/glob.ts';

const {appSubUrl} = window.config;

function initRepoSettingsCollaboration() {
  // Change collaborator access mode
  for (const dropdownEl of queryElems(document, '.page-content.repository .ui.dropdown.access-mode')) {
    const textEl = dropdownEl.querySelector(':scope > .text')!;
    const $dropdown = fomanticQuery(dropdownEl);
    $dropdown.dropdown({
      async action(text: string, value: string) {
        dropdownEl.classList.add('is-loading', 'loading-icon-2px');
        const lastValue = dropdownEl.getAttribute('data-last-value')!;
        $dropdown.dropdown('hide');
        try {
          const uid = dropdownEl.getAttribute('data-uid')!;
          await POST(dropdownEl.getAttribute('data-url')!, {data: new URLSearchParams({uid, 'mode': value})});
          textEl.textContent = text;
          dropdownEl.setAttribute('data-last-value', value);
        } catch {
          textEl.textContent = '(error)'; // prevent from misleading users when error occurs
          dropdownEl.setAttribute('data-last-value', lastValue);
        } finally {
          dropdownEl.classList.remove('is-loading');
        }
      },
      onHide() {
        // set to the really selected value, defer to next tick to make sure `action` has finished
        // its work because the calling order might be onHide -> action
        setTimeout(() => {
          const $item = $dropdown.dropdown('get item', dropdownEl.getAttribute('data-last-value'));
          if ($item) {
            $dropdown.dropdown('set selected', dropdownEl.getAttribute('data-last-value'));
          } else {
            textEl.textContent = '(none)'; // prevent from misleading users when the access mode is undefined
          }
        }, 0);
      },
    });
  }
}

function initRepoSettingsSearchTeamBox() {
  const searchTeamBox = document.querySelector('#search-team-box');
  if (!searchTeamBox) return;

  fomanticQuery(searchTeamBox).search({
    minCharacters: 2,
    searchFields: ['name', 'description'],
    showNoResults: false,
    rawResponse: true,
    apiSettings: {
      url: `${appSubUrl}/org/${searchTeamBox.getAttribute('data-org-name')}/teams/-/search?q={query}`,
      onResponse(response: any) {
        const items: Array<Record<string, any>> = [];
        for (const item of response.data) {
          items.push({
            title: item.name,
            description: `${item.permission} access`, // TODO: translate this string
          });
        }
        return {results: items};
      },
    },
  });
}

function initRepoSettingsGitHook() {
  if (!document.querySelector('.page-content.repository.settings.edit.githook')) return;
  createCodeEditor(document.querySelector<HTMLTextAreaElement>('#content')!);
}

function initRepoSettingsBranches() {
  if (!document.querySelector('.repository.settings.branches')) return;

  for (const el of document.querySelectorAll<HTMLInputElement>('.toggle-target-enabled')) {
    el.addEventListener('change', function () {
      const target = document.querySelector(this.getAttribute('data-target')!);
      target?.classList.toggle('disabled', !this.checked);
    });
  }

  for (const el of document.querySelectorAll<HTMLInputElement>('.toggle-target-disabled')) {
    el.addEventListener('change', function () {
      const target = document.querySelector(this.getAttribute('data-target')!);
      if (this.checked) target?.classList.add('disabled'); // only disable, do not auto enable
    });
  }

  document.querySelector<HTMLInputElement>('#dismiss_stale_approvals')?.addEventListener('change', function () {
    document.querySelector('#ignore_stale_approvals_box')?.classList.toggle('disabled', this.checked);
  });

  // show the `Matched` mark for the status checks that match the pattern
  const markMatchedStatusChecks = () => {
    const patterns = (document.querySelector<HTMLTextAreaElement>('#status_check_contexts')!.value || '').split(/[\r\n]+/);
    const validPatterns = patterns.map((item) => item.trim()).filter(Boolean as unknown as <T>(x: T | boolean) => x is T);
    const marks = document.querySelectorAll('.status-check-matched-mark');

    for (const el of marks) {
      let matched = false;
      const statusCheck = el.getAttribute('data-status-check')!;
      for (const pattern of validPatterns) {
        if (globMatch(statusCheck, pattern, '/')) {
          matched = true;
          break;
        }
      }
      toggleElem(el, matched);
    }
  };
  markMatchedStatusChecks();
  document.querySelector('#status_check_contexts')!.addEventListener('input', onInputDebounce(markMatchedStatusChecks));
}

function initRepoSettingsOptions() {
  const pageContent = document.querySelector('.page-content.repository.settings.options');
  if (!pageContent) return;

  // toggle related panels for the checkbox/radio inputs, the "selector" may not exist
  const toggleTargetContextPanel = (selector: string, enabled: boolean) => {
    if (!selector) return;
    queryElems(document, selector, (el) => el.classList.toggle('disabled', !enabled));
  };
  queryElems<HTMLInputElement>(pageContent, '.enable-system', (el) => el.addEventListener('change', () => {
    toggleTargetContextPanel(el.getAttribute('data-target')!, el.checked);
    toggleTargetContextPanel(el.getAttribute('data-context')!, !el.checked);
  }));
  queryElems<HTMLInputElement>(pageContent, '.enable-system-radio', (el) => el.addEventListener('change', () => {
    toggleTargetContextPanel(el.getAttribute('data-target')!, el.value === 'true');
    toggleTargetContextPanel(el.getAttribute('data-context')!, el.value === 'false');
  }));

  queryElems<HTMLInputElement>(pageContent, '.js-tracker-issue-style', (el) => el.addEventListener('change', () => {
    const checkedVal = el.value;
    pageContent.querySelector('#tracker-issue-style-regex-box')!.classList.toggle('disabled', checkedVal !== 'regexp');
  }));
}

function setPanelDisabled(panel: HTMLElement, disabled: boolean) {
  panel.hidden = disabled;
  for (const input of panel.querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>('input, select, textarea')) {
    input.disabled = disabled;
  }
}

function updatePushMirrorAuthForm(form: HTMLFormElement) {
  const editAuthTypeInput = form.querySelector<HTMLInputElement>('#push-mirror-edit-auth-type');
  const editHostKeyPolicyInput = form.querySelector<HTMLInputElement>('#push-mirror-edit-ssh-host-key-policy-value');
  const editAuthType = editAuthTypeInput?.value;
  const editHostKeyPolicy = editHostKeyPolicyInput?.value;
  const authType = editAuthType || form.querySelector<HTMLInputElement>('input[name="push_mirror_auth_type"]:checked')?.value || 'https';
  const authTypeRadio = form.querySelector<HTMLInputElement>(`input[name="push_mirror_auth_type"][value="${authType}"]`);
  if (authTypeRadio) authTypeRadio.checked = true;
  if (editAuthTypeInput) editAuthTypeInput.value = '';

  const hostKeyPolicySelect = form.querySelector<HTMLSelectElement>('select[name="push_mirror_ssh_host_key_policy"]');
  if (editHostKeyPolicy && hostKeyPolicySelect) hostKeyPolicySelect.value = editHostKeyPolicy;
  if (editHostKeyPolicyInput) editHostKeyPolicyInput.value = '';

  for (const panel of form.querySelectorAll<HTMLElement>('.js-push-mirror-auth-panel')) {
    setPanelDisabled(panel, panel.getAttribute('data-auth-type') !== authType);
  }

  const keyMode = form.querySelector<HTMLInputElement>('input[name="push_mirror_ssh_key_mode"]:checked')?.value || 'generate';
  for (const panel of form.querySelectorAll<HTMLElement>('.js-push-mirror-ssh-key-mode-panel')) {
    setPanelDisabled(panel, panel.getAttribute('data-ssh-key-mode') !== keyMode);
  }

  const publicKeyInput = form.querySelector<HTMLInputElement>('#push-mirror-edit-ssh-public-key');
  const publicKeyPanel = form.querySelector<HTMLElement>('.js-push-mirror-current-public-key');
  if (publicKeyPanel) setPanelDisabled(publicKeyPanel, authType !== 'ssh' || !publicKeyInput?.value);
}

function initRepoSettingsMirror() {
  const pageContent = document.querySelector('.page-content.repository.settings.mirror');
  if (!pageContent) return;

  for (const form of pageContent.querySelectorAll<HTMLFormElement>('.js-push-mirror-form')) {
    form.addEventListener('change', (e) => {
      const target = e.target as HTMLElement;
      if (target.matches('input[name="push_mirror_auth_type"], input[name="push_mirror_ssh_key_mode"]')) {
        updatePushMirrorAuthForm(form);
      }
    });
    updatePushMirrorAuthForm(form);
  }

  document.addEventListener('click', (e) => {
    if (!(e.target as HTMLElement).closest('.show-modal[data-modal="#push-mirror-edit-modal"]')) return;
    setTimeout(() => {
      const form = document.querySelector<HTMLFormElement>('#push-mirror-edit-modal .js-push-mirror-form');
      if (!form) return;
      form.querySelector<HTMLInputElement>('#push-mirror-edit-ssh-key-mode-keep')!.checked = true;
      form.querySelector<HTMLTextAreaElement>('#push-mirror-edit-ssh-private-key')!.value = '';
      updatePushMirrorAuthForm(form);
    }, 0);
  });
}

export function initRepoSettings() {
  if (!document.querySelector('.page-content.repository.settings')) return;
  initRepoSettingsOptions();
  initRepoSettingsBranches();
  initRepoSettingsCollaboration();
  initRepoSettingsSearchTeamBox();
  initRepoSettingsGitHook();
  initRepoSettingsMirror();
  initRepoSettingsBranchesDrag();
}
