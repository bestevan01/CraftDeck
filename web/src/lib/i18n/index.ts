import { derived, writable } from 'svelte/store';

// SSR is disabled (see +layout.ts) -- this module only ever runs in the
// browser, so localStorage/navigator are always available; no SSR guards
// needed.

export type Locale = 'ko' | 'en';

const STORAGE_KEY = 'craftdeck-locale';

// Every component-scoped dictionary (web/src/lib/i18n/dict/*.ts) is merged
// here into one flat key -> string map per locale. Splitting the source
// files by component (rather than one giant ko.ts/en.ts) is what let many
// components be converted in parallel without every edit touching the same
// file.
import { messages as accountModal } from './dict/AccountModal';
import { messages as cloudflareTutorialModal } from './dict/CloudflareTutorialModal';
import { messages as confirmDialog } from './dict/ConfirmDialog';
import { messages as consoleTab } from './dict/ConsoleTab';
import { messages as copyButton } from './dict/CopyButton';
import { messages as createInstanceModal } from './dict/CreateInstanceModal';
import { messages as domainConnectionCard } from './dict/DomainConnectionCard';
import { messages as externalAccessCard } from './dict/ExternalAccessCard';
import { messages as filesTab } from './dict/FilesTab';
import { messages as gameSettingsModal } from './dict/GameSettingsModal';
import { messages as manageTab } from './dict/ManageTab';
import { messages as memoryConflictModal } from './dict/MemoryConflictModal';
import { messages as memorySlider } from './dict/MemorySlider';
import { messages as miscSettings } from './dict/MiscSettings';
import { messages as overclockCard } from './dict/OverclockCard';
import { messages as pluginSearchModal } from './dict/PluginSearchModal';
import { messages as pluginsTab } from './dict/PluginsTab';
import { messages as reasonModal } from './dict/ReasonModal';
import { messages as resourcePanel } from './dict/ResourcePanel';
import { messages as swapCard } from './dict/SwapCard';
import { messages as tourOverlay } from './dict/TourOverlay';
import { messages as twoFactorModal } from './dict/TwoFactorModal';
import { messages as updateAvailableModal } from './dict/UpdateAvailableModal';
import { messages as updateSettingsCard } from './dict/UpdateSettingsCard';
import { messages as velocityProxyCard } from './dict/VelocityProxyCard';
import { messages as wanWarningModal } from './dict/WANWarningModal';
import { messages as common } from './dict/common';
import { messages as instanceDetailPage } from './dict/instanceDetailPage';
import { messages as loginPage } from './dict/loginPage';
import { messages as mainPage } from './dict/mainPage';

const dictModules = [
	accountModal,
	cloudflareTutorialModal,
	confirmDialog,
	consoleTab,
	copyButton,
	createInstanceModal,
	domainConnectionCard,
	externalAccessCard,
	filesTab,
	gameSettingsModal,
	manageTab,
	memoryConflictModal,
	memorySlider,
	miscSettings,
	overclockCard,
	pluginSearchModal,
	pluginsTab,
	reasonModal,
	resourcePanel,
	swapCard,
	tourOverlay,
	twoFactorModal,
	updateAvailableModal,
	updateSettingsCard,
	velocityProxyCard,
	wanWarningModal,
	common,
	instanceDetailPage,
	loginPage,
	mainPage
];

type Dict = Record<string, string>;

function mergeLocale(loc: Locale): Dict {
	const out: Dict = {};
	for (const mod of dictModules) {
		Object.assign(out, mod[loc]);
	}
	return out;
}

const dictionaries: Record<Locale, Dict> = {
	ko: mergeLocale('ko'),
	en: mergeLocale('en')
};

function detectLocale(): Locale {
	const saved = localStorage.getItem(STORAGE_KEY);
	if (saved === 'ko' || saved === 'en') return saved;
	return navigator.language.toLowerCase().startsWith('ko') ? 'ko' : 'en';
}

export const locale = writable<Locale>(detectLocale());

export function setLocale(l: Locale) {
	locale.set(l);
	localStorage.setItem(STORAGE_KEY, l);
}

// {name} placeholders inside a message get substituted from vars -- used
// for the handful of strings that embed a dynamic value (counts, names).
export const t = derived(locale, ($locale) => {
	const dict = dictionaries[$locale];
	return (key: string, vars?: Record<string, string | number>) => {
		let str = dict[key] ?? dictionaries.ko[key] ?? key;
		if (vars) {
			for (const [k, v] of Object.entries(vars)) {
				str = str.replaceAll(`{${k}}`, String(v));
			}
		}
		return str;
	};
});
