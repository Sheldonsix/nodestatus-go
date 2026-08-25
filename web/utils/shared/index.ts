export const parseLoad = (num: number): number => {
  const load = Number(num.toFixed(2));
  return load % 1 ? load : Math.round(load);
};

export const parseUpdateTime = (updated: number | undefined): string => {
  if (!updated) return '从未';
  const nowTime: number = Date.now() / 1000;
  const seconds: number = Math.floor(nowTime - updated);
  let interval = Math.floor(seconds / 31536000);
  if (interval > 1) return `${interval} 年前.`;
  interval = Math.floor(seconds / 2592000);
  if (interval > 1) return `${interval} 月前`;
  interval = Math.floor(seconds / 86400);
  if (interval > 1) return `${interval} 日前`;
  interval = Math.floor(seconds / 3600);
  if (interval > 1) return `${interval} 小时前`;
  interval = Math.floor(seconds / 60);
  if (interval > 1) return `${interval} 分钟前`;
  return '几秒前';
};

export const parseUptime = (uptime: number): string => {
  if (uptime >= 86400) return `${Math.floor(uptime / 86400)} 天`;

  const h = String(Math.floor(uptime / 3600)).padStart(2, '0');
  const m = String(Math.floor((uptime / 60) % 60)).padStart(2, '0');
  const s = String(Math.floor(uptime % 60)).padStart(2, '0');
  return `${h}:${m}:${s}`;
};

export const formatNetwork = (data: number | null | undefined): string => {
  if (data === null) return '-';
  if (data === undefined) return '0B';
  if (data < 1024) return `${data.toFixed(0)}B`;
  if (data < 1024 * 1024) return `${(data / 1024).toFixed(0)}K`;
  if (data < 1024 * 1024 * 1024) return `${(data / 1024 / 1024).toFixed(1)}M`;
  if (data < 1024 * 1024 * 1024 * 1024) return `${(data / 1024 / 1024 / 1024).toFixed(2)}G`;
  return `${(data / 1024 / 1024 / 1024 / 1024).toFixed(2)}T`;
};

export const formatByte = (data: number | undefined): string => {
  if (data === undefined) return '0 B';
  if (data < 1024) return `${data.toFixed(0)} B`;
  if (data < 1024 * 1024) return `${(data / 1024).toFixed(2)} KiB`;
  if (data < 1024 * 1024 * 1024) return `${(data / 1024 / 1024).toFixed(2)} MiB`;
  if (data < 1024 * 1024 * 1024 * 1024) return `${(data / 1024 / 1024 / 1024).toFixed(2)} GiB`;
  return `${(data / 1024 / 1024 / 1024 / 1024).toFixed(2)} TiB`;
};
