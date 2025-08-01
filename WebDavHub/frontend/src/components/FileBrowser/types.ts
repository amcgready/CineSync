// Type definitions for FileBrowser

export type SortOption = 'name-asc' | 'name-desc' | 'modified-desc' | 'modified-asc' | 'size-desc' | 'size-asc';

export interface FileItem {
  name: string;
  type: 'file' | 'directory';
  size?: string;
  modified?: string;
  path?: string;
  webdavPath?: string;
  sourcePath?: string;
  destinationPath?: string;
  fullPath?: string;
  hasSeasonFolders?: boolean;
  isSeasonFolder?: boolean;
  isCategoryFolder?: boolean;
  tmdbId?: string;
  mediaType?: 'movie' | 'tv';
  posterPath?: string;
  title?: string;
  releaseDate?: string;
  firstAirDate?: string;
  isSourceRoot?: boolean;
  isSourceFile?: boolean;
  isMediaFile?: boolean;
  processingStatus?: string;
  seasonNumber?: number | null;
  lastProcessedAt?: number | null;
}

export interface MobileListItemProps {
  file: FileItem;
  onItemClick: () => void;
  onMenuClick?: (event: React.MouseEvent<HTMLElement>) => void;
  formatDate: (date?: string) => string;
  menu?: React.ReactNode;
  isSelected?: boolean;
  onSelect?: (checked: boolean) => void;
  showSelection?: boolean;
}

export interface AlphabetIndexProps {
  selectedLetter: string | null;
  onLetterClick: (letter: string | null) => void;
  loading?: boolean;
}