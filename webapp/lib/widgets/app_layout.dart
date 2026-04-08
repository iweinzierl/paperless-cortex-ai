import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:webapp/theme.dart';

class AppLayout extends StatelessWidget {
  final Widget child;
  const AppLayout({Key? key, required this.child}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: TailwindColors.surface,
      body: Row(
        children: [
          const SideNavBar(),
          Expanded(
            child: Column(
              children: [
                const TopNavBar(),
                Expanded(child: child),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class SideNavBar extends StatelessWidget {
  const SideNavBar({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    final location = GoRouterState.of(context).matchedLocation;
    return Container(
      width: 256,
      color: TailwindColors.surfaceContainerLow,
      padding: const EdgeInsets.symmetric(vertical: 24, horizontal: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Cortex Graphite',
                  style: TextStyle(
                    fontSize: 20,
                    fontWeight: FontWeight.w900,
                    color: TailwindColors.onSurface,
                    letterSpacing: -0.5,
                  ),
                ),
                Text(
                  'LLM Instance: Active',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    color: TailwindColors.outline,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 32),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(vertical: 12),
            decoration: BoxDecoration(
              gradient: const LinearGradient(
                colors: [
                  TailwindColors.primary,
                  TailwindColors.primaryContainer,
                ],
              ),
              borderRadius: BorderRadius.circular(12),
              boxShadow: [
                BoxShadow(
                  color: TailwindColors.primary.withValues(alpha: 0.3),
                  blurRadius: 4,
                  offset: const Offset(0, 2),
                ),
              ],
            ),
            child: const Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Icon(Icons.add, color: TailwindColors.onPrimary, size: 20),
                SizedBox(width: 8),
                Text(
                  'New Prompt',
                  style: TextStyle(
                    color: TailwindColors.onPrimary,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 32),
          _NavItem(
            icon: Icons.dashboard,
            title: 'Dashboard',
            active: location.startsWith('/dashboard'),
            onTap: () => context.go('/dashboard'),
          ),
          const SizedBox(height: 4),
          _NavItem(
            icon: Icons.queue,
            title: 'Queue',
            active: location.startsWith('/queue'),
            onTap: () => context.go('/queue'),
          ),
          const SizedBox(height: 4),
          _NavItem(
            icon: Icons.settings,
            title: 'Configuration',
            active: location.startsWith('/configuration'),
            onTap: () => context.go('/configuration'),
          ),
          const Spacer(),
          const Divider(color: TailwindColors.outlineVariant),
          const SizedBox(height: 16),
          _NavItem(
            icon: Icons.help_outline,
            title: 'Docs',
            active: false,
            onTap: () {},
          ),
          const SizedBox(height: 4),
          _NavItem(
            icon: Icons.logout,
            title: 'Logout',
            active: false,
            onTap: () => context.go('/login'),
          ),
        ],
      ),
    );
  }
}

class _NavItem extends StatelessWidget {
  final IconData icon;
  final String title;
  final bool active;
  final VoidCallback onTap;

  const _NavItem({
    Key? key,
    required this.icon,
    required this.title,
    required this.active,
    required this.onTap,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(8),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: active
              ? TailwindColors.primary.withValues(alpha: 0.1)
              : Colors.transparent,
          borderRadius: BorderRadius.circular(8),
          border: active
              ? const Border(
                  left: BorderSide(color: TailwindColors.primary, width: 2),
                )
              : null,
        ),
        child: Row(
          children: [
            Icon(
              icon,
              size: 20,
              color: active ? TailwindColors.primary : TailwindColors.outline,
            ),
            const SizedBox(width: 12),
            Text(
              title,
              style: TextStyle(
                fontSize: 14,
                fontWeight: active ? FontWeight.w600 : FontWeight.w500,
                color: active ? TailwindColors.primary : TailwindColors.outline,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class TopNavBar extends StatelessWidget {
  const TopNavBar({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 64,
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.8),
        border: const Border(
          bottom: BorderSide(color: TailwindColors.surfaceContainerHigh),
        ),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 32),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          // Search
          SizedBox(
            width: 400,
            child: TextField(
              decoration: InputDecoration(
                hintText: 'Search prompts, documents, or models...',
                hintStyle: const TextStyle(
                  color: TailwindColors.outline,
                  fontSize: 14,
                ),
                prefixIcon: const Icon(
                  Icons.search,
                  color: TailwindColors.outline,
                  size: 20,
                ),
                filled: true,
                fillColor: TailwindColors.surfaceContainerHighest,
                contentPadding: const EdgeInsets.symmetric(vertical: 0),
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(24),
                  borderSide: BorderSide.none,
                ),
              ),
            ),
          ),

          // Profiles and Actions
          Row(
            children: [
              IconButton(
                onPressed: () {},
                icon: const Icon(
                  Icons.notifications_none,
                  color: TailwindColors.outline,
                ),
              ),
              IconButton(
                onPressed: () {},
                icon: const Icon(
                  Icons.dns_outlined,
                  color: TailwindColors.outline,
                ),
              ),
              Container(
                height: 32,
                width: 1,
                color: TailwindColors.surfaceContainerHigh,
                margin: const EdgeInsets.symmetric(horizontal: 8),
              ),
              const Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text(
                    'Administrator',
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.bold,
                      color: TailwindColors.onSurface,
                    ),
                  ),
                  Text(
                    'Node: Online',
                    style: TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w500,
                      color: TailwindColors.tertiary,
                    ),
                  ),
                ],
              ),
              const SizedBox(width: 8),
              Container(
                width: 32,
                height: 32,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: TailwindColors.surfaceContainerHighest,
                  border: Border.all(color: TailwindColors.outlineVariant),
                ),
                child: const Icon(
                  Icons.person,
                  size: 20,
                  color: TailwindColors.outline,
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
