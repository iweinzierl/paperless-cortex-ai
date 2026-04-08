import 'package:flutter/material.dart';
import 'package:webapp/services/api_service.dart';

class AuthProvider extends ChangeNotifier {
  final ApiService apiService;

  bool _initialized = false;
  bool _isAuthenticated = false;
  String? _username;

  bool get initialized => _initialized;
  bool get isAuthenticated => _isAuthenticated;
  String? get username => _username;

  AuthProvider(this.apiService);

  Future<void> init() async {
    await apiService.init();
    if (apiService.token != null && apiService.token!.isNotEmpty) {
      try {
        final session = await apiService.getMe();
        _isAuthenticated = true;
        _username = session.username;
      } catch (e) {
        _isAuthenticated = false;
        await apiService.logout();
      }
    }
    _initialized = true;
    notifyListeners();
  }

  Future<void> login(String username, String password) async {
    final session = await apiService.login(username, password);
    _isAuthenticated = true;
    _username = session.username;
    notifyListeners();
  }

  Future<void> logout() async {
    await apiService.logout();
    _isAuthenticated = false;
    _username = null;
    notifyListeners();
  }
}
