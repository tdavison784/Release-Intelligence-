.. only:: not (epub or latex or html)

    WARNING: You are looking at unreleased Cilium documentation.
    Please use the official rendered version released here:
    https://docs.cilium.io

.. _admin_upgrade:

*************
Upgrade Guide
*************

This upgrade guide is intended for Cilium running on Kubernetes.

Running pre-flight check (Required)
===================================

When rolling out an upgrade with Kubernetes, Kubernetes will first terminate
the pod followed by pulling the new image version.

.. tabs::
  .. group-tab:: kubectl

    .. cilium-helm-template::
       :namespace: kube-system
       :set: preflight.enabled=true

Version Specific Notes
======================

This section details the upgrade notes specific to 1.19.

Kubernetes Compatibility
------------------------

All Kubernetes versions listed are e2e tested and guaranteed to be compatible
with Cilium.

+------------------------+---------------------------+----------------------------------+
| k8s Version            | k8s NetworkPolicy API     | CiliumNetworkPolicy              |
+------------------------+---------------------------+----------------------------------+
|                        |                           | ``cilium.io/v2`` has a           |
| 1.29, 1.30, 1.31, 1.32 | * `networking.k8s.io/v1`_ | :term:`CustomResourceDefinition` |
+------------------------+---------------------------+----------------------------------+

1.19 Upgrade Notes
------------------

Action Required
~~~~~~~~~~~~~~~

If you are using the following features in your environment, then you may need
to take action because of changes to the behavior of these features.

* The ``v2alpha1`` version of the ``CiliumNodeConfig`` CRD is deprecated.
  Update manifests to use ``cilium.io/v2`` instead.
* Cilium's Gateway API support now requires Gateway API v1.6.1 at a minimum.

Informational Notes
~~~~~~~~~~~~~~~~~~~

* This Cilium version now requires a v5.10 Linux kernel or newer.
* The support for Envoy Go Extensions (proxylib) is deprecated, and will be
  removed in a future release.

Changes to Features
~~~~~~~~~~~~~~~~~~~

* When using ``KubeProxyReplacement`` for Service Loadbalancing, in-cluster
  connections to NodePort services by regular pods are now immediately
  load-balanced when network traffic leaves the client pod.

Deprecated Options
##################

The following options have been deprecated in this version of Cilium:

* The ``hubble.preferIpv6`` Helm value and ``--hubble-prefer-ipv6`` agent flag
  have been deprecated. Use the top-level ``preferIPv6`` value instead.

Removed Options
###############

* The previously deprecated ``--k8s-api-server`` agent flag has been removed in
  favor of ``--k8s-api-server-urls``.

Advanced
========

Upgrade Impact
--------------

Existing connections may be disrupted during upgrade.
