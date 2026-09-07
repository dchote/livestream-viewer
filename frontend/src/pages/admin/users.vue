<template>
  <v-container class="page-content">
    <StandardCard>
      <template #header>
        <span class="text-h5 header-title">System Users</span>
        <v-spacer />
        <v-btn
          color="primary"
          variant="elevated"
          size="small"
          @click="showCreateDialog = true"
        >
          <v-icon start size="small">mdi-plus</v-icon>
          Add User
        </v-btn>
      </template>

      <v-alert v-if="error" type="error" density="compact" class="mb-4">{{ error }}</v-alert>

      <v-data-table
        v-if="users.length > 0"
        :headers="headers"
        :items="users"
        density="comfortable"
        :items-per-page="50"
      >
        <template #item.role="{ item }">
          {{ roleLabel(item.role) }}
        </template>
        <template #item.actions="{ item }">
          <v-menu location="bottom end">
            <template #activator="{ props: menuProps }">
              <v-btn
                v-bind="menuProps"
                icon="mdi-dots-vertical"
                variant="text"
                size="small"
                class="px-2"
              />
            </template>
            <v-list density="compact">
              <v-tooltip
                v-if="item.id === currentUserId"
                location="top"
                text="You cannot edit your own role"
              >
                <template #activator="{ props: tooltipProps }">
                  <v-list-item
                    v-bind="tooltipProps"
                    prepend-icon="mdi-pencil"
                    title="Edit role"
                    disabled
                  />
                </template>
              </v-tooltip>
              <v-list-item
                v-else
                prepend-icon="mdi-pencil"
                title="Edit role"
                @click="openEditRole(item)"
              />
              <v-tooltip
                v-if="item.id === currentUserId"
                location="top"
                text="You cannot delete yourself"
              >
                <template #activator="{ props: tooltipProps }">
                  <v-list-item
                    v-bind="tooltipProps"
                    prepend-icon="mdi-delete"
                    title="Delete"
                    disabled
                  />
                </template>
              </v-tooltip>
              <v-list-item
                v-else
                prepend-icon="mdi-delete"
                title="Delete"
                @click="openDelete(item)"
              />
            </v-list>
          </v-menu>
        </template>
        <template #bottom />
      </v-data-table>
      <div v-else class="empty-state">
        <v-icon class="empty-state__icon">mdi-account-multiple</v-icon>
        <div class="text-h6 empty-state__title">No users yet</div>
        <p class="empty-state__copy">Add a management user to share access to this appliance.</p>
      </div>
    </StandardCard>

    <CreateUserDialog
      ref="createDialog"
      v-model="showCreateDialog"
      :loading="creating"
      @save="confirmCreate"
    />

    <EditUserRoleDialog
      ref="editDialog"
      v-model="showEditRoleDialog"
      :user="editingUser"
      :loading="updatingRole"
      @save="confirmEditRole"
      @close="editingUser = null"
    />

    <StandardDialog
      v-model="showDeleteDialog"
      title="Delete user?"
      max-width="400"
      :fullscreen="mobile"
      @close="userToDelete = null"
    >
      <v-alert v-if="deleteError" type="error" density="compact" class="mb-4">
        {{ deleteError }}
      </v-alert>
      <p>Are you sure you want to delete {{ userToDelete?.username }}?</p>
      <template #actions>
        <v-spacer />
        <v-btn variant="text" class="mr-2" @click="showDeleteDialog = false">Cancel</v-btn>
        <v-btn color="error" variant="elevated" :loading="deleting" @click="confirmDelete">
          Delete
        </v-btn>
      </template>
    </StandardDialog>
  </v-container>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useDisplay } from 'vuetify'
import StandardCard from '@/components/common/StandardCard.vue'
import StandardDialog from '@/components/common/StandardDialog.vue'
import EditUserRoleDialog from '@/components/admin/EditUserRoleDialog.vue'
import CreateUserDialog from '@/components/admin/CreateUserDialog.vue'
import { api } from '@/utils/api'
import { roleLabel } from '@/utils/roles'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const { mobile } = useDisplay()
const users = ref([])
const error = ref('')
const createDialog = ref(null)
const editDialog = ref(null)
const showEditRoleDialog = ref(false)
const editingUser = ref(null)
const updatingRole = ref(false)
const showDeleteDialog = ref(false)
const userToDelete = ref(null)
const deleting = ref(false)
const deleteError = ref('')
const showCreateDialog = ref(false)
const creating = ref(false)

const currentUserId = computed(() => auth.user?.id)

const headers = [
  { title: 'Username', key: 'username' },
  { title: 'Role', key: 'role' },
  { title: '', key: 'actions', sortable: false, align: 'end', width: 56 },
]

onMounted(async () => {
  try {
    const data = await api.get('api/v1/users')
    users.value = data.users || []
  } catch (e) {
    console.log('[Users] API error:', e)
    error.value = e.message || 'Failed to load users'
  }
})

async function confirmCreate(payload) {
  creating.value = true
  try {
    const created = await api.post('api/v1/users', payload)
    users.value = [...users.value, created]
    showCreateDialog.value = false
  } catch (e) {
    console.log('[Users] create error:', e)
    createDialog.value?.setError(e.message || 'Failed to create user')
  } finally {
    creating.value = false
  }
}

function openEditRole(u) {
  if (u.id === currentUserId.value) return
  editingUser.value = { ...u }
  showEditRoleDialog.value = true
}

async function confirmEditRole(payload) {
  updatingRole.value = true
  try {
    await api.patch(`api/v1/users/${payload.id}`, { role: payload.role })
    const u = users.value.find((x) => x.id === payload.id)
    if (u) u.role = payload.role
    showEditRoleDialog.value = false
    editingUser.value = null
  } catch (e) {
    console.log('[Users] update role error:', e)
    editDialog.value?.setError(e.message || 'Failed to update role')
  } finally {
    updatingRole.value = false
  }
}

function openDelete(u) {
  userToDelete.value = u
  deleteError.value = ''
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!userToDelete.value) return
  deleteError.value = ''
  deleting.value = true
  try {
    await api.delete(`api/v1/users/${userToDelete.value.id}`)
    users.value = users.value.filter((x) => x.id !== userToDelete.value.id)
    showDeleteDialog.value = false
    userToDelete.value = null
  } catch (e) {
    console.log('[Users] delete error:', e)
    deleteError.value = e.message || 'Failed to delete user'
  } finally {
    deleting.value = false
  }
}
</script>
