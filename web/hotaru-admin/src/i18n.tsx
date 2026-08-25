import type { FC, ReactNode } from 'react';
import { createContext, useContext, useMemo, useState } from 'react';

type Lang = 'zh' | 'en';

const messages = {
  zh: {
    dashboard: '仪表盘',
    management: '节点管理',
    incidents: '故障记录',
    settings: '设置',
    logout: '退出登录',
    login: '登录',
  },
  en: {
    dashboard: 'Dashboard',
    management: 'Management',
    incidents: 'Incidents',
    settings: 'Settings',
    logout: 'Logout',
    login: 'Login',
  },
};

type MessageKey = keyof typeof messages['zh'];

const I18nContext = createContext<{
  lang: Lang;
  setLang: (lang: Lang) => void;
  t: (key: MessageKey) => string;
} | null>(null);

function getInitialLang() {
  const lang = localStorage.getItem('lang');
  return lang === 'en' || lang === 'zh' ? lang : 'zh';
}

export const I18nProvider: FC<{ children: ReactNode }> = ({ children }) => {
  const [lang, setLangState] = useState<Lang>(getInitialLang);

  const setLang = (nextLang: Lang) => {
    setLangState(nextLang);
    localStorage.setItem('lang', nextLang);
  };

  const value = useMemo(() => ({
    lang,
    setLang,
    t: (key: MessageKey) => messages[lang][key],
  }), [lang]);

  return (
    <I18nContext.Provider value={value}>
      {children}
    </I18nContext.Provider>
  );
};

export function useI18n() {
  const ctx = useContext(I18nContext);
  if (!ctx) {
    throw new Error('useI18n must be used within a I18nProvider');
  }
  return ctx;
}

export default I18nContext;
