import { computed, toRefs } from 'vue';

import { ServerItem } from '../../types';
import {
  formatByte as formatByteValue,
  formatNetwork as formatNetworkValue,
  parseLoad,
  parseUptime
} from '../../shared';

interface Props {
  server: ServerItem;
}

const useStatus = (props: Props) => {
  const { server } = toRefs(props);
  const getStatus = computed((): boolean => server.value.status.online4 || server.value.status.online6);

  const getLoad = computed((): number => parseLoad(server.value.status.load));

  const getCpuStatus = computed(
    (): number => (
      server.value.status.cpu === undefined
        ? 100
        : Math.round(server.value.status.cpu)
    )
  );

  const getNetworkProtocol = computed((): string => {
    if (server.value.status.online4 && server.value.status.online6) {
      return '双栈';
    }
    if (server.value.status.online4) {
      return 'IPv4';
    }
    if (server.value.status.online6) {
      return 'IPv6';
    }
    return '维护中';
  });

  const getRAMStatus = computed(
    (): number => (
      server.value.status.memory_total === undefined
        ? 100
        : Math.round(((server.value.status.memory_used / server.value.status.memory_total) * 100))
    )
  );

  const getHDDStatus = computed(
    (): number => (server.value.status.hdd_total === undefined
      ? 100
      : Math.round(((server.value.status.hdd_used / server.value.status.hdd_total) * 100))
    )
  );

  const getProcessBarStatus = computed(
    () => (data: number) => {
      if (data > 90) return 'error';
      if (data > 70) return 'warning';
      return 'success';
    }
  );

  const getUpTime = computed((): string => {
    if (getStatus.value) {
      const { uptime } = server.value.status;
      return parseUptime(uptime);
    }
    return '-';
  });

  const formatNetwork = computed(() => formatNetworkValue);

  const formatByte = computed(() => formatByteValue);

  return {
    getStatus,
    getNetworkProtocol,
    getLoad,
    getCpuStatus,
    getRAMStatus,
    getHDDStatus,
    getProcessBarStatus,
    getUpTime,
    formatNetwork,
    formatByte
  };
};
export default useStatus;
