type Upload = {
  id: string;
  status: string;
  serverID?: string;
  scan?: UploadScan;
  message?: string;
  canRetry?: boolean;
  modified?: string;
  path: string;
  name: string;
  file: File | null;
  type: ResourceType;
  overwrite: boolean;
  totalBytes: number;
  sentBytes: number;
  rawProgress: {
    sentBytes: number;
  };
};

interface UploadEntry {
  name: string;
  size: number;
  isDir: boolean;
  fullPath?: string;
  to?: string;
  file?: File;
  overwrite?: boolean;
}

type UploadList = UploadEntry[];

interface UploadScan {
  status: string;
  reason?: string;
  message?: string;
}
interface ServerUpload {
  id: string;
  path: string;
  name: string;
  length: number;
  offset: number;
  status: string;
  scan: UploadScan;
  message?: string;
  sha256?: string;
  modified?: string;
  createdAt: string;
  updatedAt: string;
  expiresAt: string;
  canRetry: boolean;
}
