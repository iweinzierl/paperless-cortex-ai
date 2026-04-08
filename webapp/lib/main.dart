import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:provider/provider.dart';

import 'package:webapp/theme.dart';
import 'package:webapp/services/api_service.dart';
import 'package:webapp/providers/auth_provider.dart';

import 'package:webapp/widgets/app_layout.dart';
import 'package:webapp/screens/dashboard_screen.dart';
import 'package:webapp/screens/configuration_screen.dart';
import 'package:webapp/screens/queue_screen.dart';
import 'package:webapp/screens/login_screen.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  final apiService = ApiService();
  final authProvider = AuthProvider(apiService);
  await authProvider.init();

  runApp(
    MultiProvider(
      providers: [
        Provider<ApiService>.value(value: apiService),
        ChangeNotifierProvider<AuthProvider>.value(value: authProvider),
      ],
      child: const PaperlessAiExtApp(),
    ),
  );
}

class PaperlessAiExtApp extends StatelessWidget {
  const PaperlessAiExtApp({super.key});

  @override
  Widget build(BuildContext context) {
    final authProvider = context.read<AuthProvider>();

    final router = GoRouter(
      initialLocation: '/dashboard',
      refreshListenable: authProvider,
      redirect: (BuildContext context, GoRouterState state) {
        final bool loggedIn = authProvider.isAuthenticated;
        final bool isLogin = state.matchedLocation == '/login';

        if (!loggedIn && !isLogin) return '/login';
        if (loggedIn && isLogin) return '/dashboard';

        return null;
      },
      routes: <RouteBase>[
        GoRoute(
          path: '/login',
          builder: (BuildContext context, GoRouterState state) {
            return const LoginScreen();
          },
        ),
        ShellRoute(
          builder: (BuildContext context, GoRouterState state, Widget child) {
            return AppLayout(child: child);
          },
          routes: <RouteBase>[
            GoRoute(
              path: '/dashboard',
              builder: (BuildContext context, GoRouterState state) {
                return const DashboardScreen();
              },
            ),
            GoRoute(
              path: '/queue',
              builder: (BuildContext context, GoRouterState state) {
                return const QueueScreen();
              },
            ),
            GoRoute(
              path: '/configuration',
              builder: (BuildContext context, GoRouterState state) {
                return const ConfigurationScreen();
              },
            ),
          ],
        ),
      ],
    );

    return MaterialApp.router(
      title: 'Cortex Graphite',
      theme: buildAppTheme(),
      routerConfig: router,
      debugShowCheckedModeBanner: false,
    );
  }
}
