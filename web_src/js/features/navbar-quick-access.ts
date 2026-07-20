import {createApp} from 'vue';
import NavbarQuickAccess from '../components/NavbarQuickAccess.vue';

export function initNavbarQuickAccess() {
  const el = document.querySelector('#navbar-quick-access');
  if (el) createApp(NavbarQuickAccess).mount(el);
}
