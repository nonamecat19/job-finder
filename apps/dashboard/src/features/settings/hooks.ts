import { useMutation } from '@tanstack/react-query';
import { api } from '../../api';

export function useBootstrapExtension() {
  return useMutation({
    mutationFn: () => api.ext.bootstrap(),
  });
}
