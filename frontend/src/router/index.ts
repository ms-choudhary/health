import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'home', component: () => import('@/views/Home.vue') },
  {
    path: '/user/:id',
    name: 'user',
    component: () => import('@/views/UserPage.vue'),
    props: (route) => ({ userId: Number(route.params.id) }),
  },
  {
    path: '/library',
    name: 'library',
    component: () => import('@/views/FoodLibrary.vue'),
  },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})
