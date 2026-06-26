import { Component, OnInit, inject, ChangeDetectorRef, HostListener } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpClient } from '@angular/common/http';
import { RouterModule } from '@angular/router';

export interface StorageArea {
  id: string;
  kind: 'segments';
  path: string;
  enabled: boolean;
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
  utilization_pct: number;
  status: string;
  error?: string;
  eligible_segments: boolean;
}

export interface StorageResponse {
  storage_min_free_bytes: number;
  areas: StorageArea[];
}

@Component({
  selector: 'app-storage-admin',
  standalone: true,
  imports: [CommonModule, FormsModule, RouterModule],
  templateUrl: './storage-admin.component.html',
  styleUrl: './storage-admin.component.css',
})
export class StorageAdminComponent implements OnInit {
  private http = inject(HttpClient);
  private cdr = inject(ChangeDetectorRef);

  protected readonly Math = Math;

  areas: StorageArea[] = [];
  minFreeBytes = 0; // Current configuration in bytes
  minFreeGB = 0; // Slider value in GB

  totalCapacityBytes = 0;
  totalUsedBytes = 0;
  totalFreeBytes = 0;
  totalUtilizationPct = 0;

  loading = true;
  error = '';
  isSavingConfig = false;

  // Add Storage Modal State
  isAddModalOpen = false;
  newPath = '';
  newEnabled = true;
  isSaving = false;
  modalError = '';

  // Directory autocomplete / browsing
  fsItems: string[] = [];
  browsingPath = '/';
  isBrowsing = false;

  // UI dropdown and editing states
  activeDropdownAreaId: string | null = null;
  editingAreaId: string | null = null;
  editingPathValue = '';

  ngOnInit(): void {
    this.fetchData();
  }

  fetchData(): void {
    this.loading = true;
    this.error = '';

    this.http.get<StorageResponse>('/api/v1/storage-areas').subscribe({
      next: (res) => {
        this.areas = res.areas || [];
        this.minFreeBytes = res.storage_min_free_bytes;
        this.minFreeGB = Math.round(res.storage_min_free_bytes / (1024 * 1024 * 1024));
        this.calculateAggregates();
        this.loading = false;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.error = 'Failed to load storage data.';
        this.loading = false;
        this.cdr.detectChanges();
      },
    });
  }

  calculateAggregates(): void {
    let total = 0;
    let used = 0;
    let free = 0;

    // Sum details from enabled and active areas
    this.areas.forEach((area) => {
      if (area.enabled) {
        total += area.total_bytes;
        used += area.used_bytes;
        free += area.free_bytes;
      }
    });

    this.totalCapacityBytes = total;
    this.totalUsedBytes = used;
    this.totalFreeBytes = free;
    this.totalUtilizationPct = total > 0 ? Math.round((used / total) * 100) : 0;
  }

  // Action: Save global storage config
  saveConfig(): void {
    this.isSavingConfig = true;
    const bytesValue = this.minFreeGB * 1024 * 1024 * 1024;

    this.http
      .put('/api/v1/settings', {
        storage_min_free_bytes: String(bytesValue),
      })
      .subscribe({
        next: () => {
          this.isSavingConfig = false;
          this.minFreeBytes = bytesValue;
          this.fetchData();
        },
        error: (err) => {
          this.isSavingConfig = false;
          alert(`Failed to save configuration: ${err.error?.error || err.message}`);
          this.cdr.detectChanges();
        },
      });
  }

  // Action: Toggle storage area state
  toggleAreaEnabled(area: StorageArea, event?: MouseEvent): void {
    if (event) event.stopPropagation();
    this.activeDropdownAreaId = null;

    this.http
      .put(`/api/v1/storage-areas/${area.id}`, {
        enabled: !area.enabled,
      })
      .subscribe({
        next: () => {
          this.fetchData();
        },
        error: (err) => {
          alert(`Failed to update storage area: ${err.error?.error || err.message}`);
        },
      });
  }

  // Action: Delete storage area
  deleteArea(areaId: string, event?: MouseEvent): void {
    if (event) event.stopPropagation();
    this.activeDropdownAreaId = null;

    if (
      confirm(
        'Are you sure you want to remove this storage path? Existing transcode files will remain on disk but Prism will no longer read/write from this path.',
      )
    ) {
      this.http.delete(`/api/v1/storage-areas/${areaId}`).subscribe({
        next: () => {
          this.fetchData();
        },
        error: (err) => {
          alert(`Failed to delete storage area: ${err.error?.error || err.message}`);
        },
      });
    }
  }

  // Action: Start inline path editing
  startEditPath(area: StorageArea, event?: MouseEvent): void {
    if (event) event.stopPropagation();
    this.activeDropdownAreaId = null;
    this.editingAreaId = area.id;
    this.editingPathValue = area.path;
  }

  cancelEditPath(): void {
    this.editingAreaId = null;
  }

  saveEditedPath(areaId: string): void {
    if (!this.editingPathValue.trim()) return;

    this.http
      .put(`/api/v1/storage-areas/${areaId}`, {
        path: this.editingPathValue.trim(),
      })
      .subscribe({
        next: () => {
          this.editingAreaId = null;
          this.fetchData();
        },
        error: (err) => {
          alert(`Failed to update path: ${err.error?.error || err.message}`);
        },
      });
  }

  // Modal actions
  openAddModal(): void {
    this.isAddModalOpen = true;
    this.newPath = '';
    this.newEnabled = true;
    this.modalError = '';
    this.fsItems = [];
    this.browsingPath = '/';
    this.browseDir(this.browsingPath);
  }

  closeAddModal(): void {
    this.isAddModalOpen = false;
  }

  saveNewStorageArea(): void {
    if (!this.newPath.trim()) {
      this.modalError = 'Storage path is required.';
      return;
    }
    this.isSaving = true;
    this.modalError = '';

    const body = {
      kind: 'segments',
      path: this.newPath.trim(),
      enabled: this.newEnabled,
    };

    this.http.post('/api/v1/storage-areas', body).subscribe({
      next: () => {
        this.isSaving = false;
        this.isAddModalOpen = false;
        this.fetchData();
      },
      error: (err) => {
        this.isSaving = false;
        this.modalError = err.error?.error || 'Failed to add storage path.';
        this.cdr.detectChanges();
      },
    });
  }

  // FS Browsing logic
  browseDir(path: string): void {
    this.isBrowsing = true;
    let targetPath = path;
    if (targetPath !== '/' && !targetPath.endsWith('/')) {
      targetPath += '/';
    }
    this.http.get<any>(`/api/v1/fs:browse?path=${encodeURIComponent(targetPath)}`).subscribe({
      next: (res) => {
        this.browsingPath = path;
        this.fsItems = res && res.dirs ? res.dirs : [];
        this.isBrowsing = false;
        this.cdr.detectChanges();
      },
      error: () => {
        this.isBrowsing = false;
        this.cdr.detectChanges();
      },
    });
  }

  selectBrowsedPath(path: string): void {
    this.newPath = path;
    this.browseDir(path);
  }

  browseParentDir(): void {
    if (this.browsingPath === '/' || !this.browsingPath) return;
    const parts = this.browsingPath.split('/');
    parts.pop();
    const parent = parts.join('/') || '/';
    this.browseDir(parent);
  }

  // UI helpers
  toggleDropdown(areaId: string, event: MouseEvent): void {
    event.stopPropagation();
    if (this.activeDropdownAreaId === areaId) {
      this.activeDropdownAreaId = null;
    } else {
      this.activeDropdownAreaId = areaId;
    }
  }

  @HostListener('document:click', ['$event'])
  closeAllDropdowns(event: MouseEvent): void {
    this.activeDropdownAreaId = null;
  }

  formatBytes(bytes: number, decimals: number = 1): string {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
  }

  getStatusClass(status: string): string {
    switch (status.toLowerCase()) {
      case 'ok':
        return 'status-healthy';
      case 'below_reserve':
        return 'status-warning';
      case 'disabled':
        return 'status-disabled';
      default:
        return 'status-error';
    }
  }

  getStatusText(area: StorageArea): string {
    if (!area.enabled) return 'Disabled';
    switch (area.status.toLowerCase()) {
      case 'ok':
        return 'Healthy';
      case 'below_reserve':
        return 'Low Space';
      case 'missing':
        return 'Path Missing';
      case 'permission_denied':
        return 'Permission Denied';
      case 'unwritable':
        return 'Unwritable';
      default:
        return area.error || 'Stat Error';
    }
  }
}
