import type { FC } from 'react';
import { ConfigProvider } from 'antd';
import enUS from 'antd/lib/locale/en_US';

import zhCN from 'antd/lib/locale/zh_CN';
import React from 'react';
import ReactDOM from 'react-dom';
import App from './App';
import { I18nProvider, useI18n } from './i18n';

import 'virtual:svg-icons-register';
/* Ant Design */
import 'antd/dist/antd.css';
/* UnoCSS */
import 'virtual:uno.css';
import '@unocss/reset/tailwind-compat.css';

const Root: FC = () => {
  const { lang } = useI18n();
  return (
    <ConfigProvider locale={lang === 'zh' ? zhCN : enUS}>
      <App />
    </ConfigProvider>
  );
};

ReactDOM.render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN}>
      <I18nProvider>
        <Root />
      </I18nProvider>
    </ConfigProvider>
  </React.StrictMode>,
  document.getElementById('root'),
);
